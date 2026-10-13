package apiserver

import (
	"context"
	"math"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	auth "github.com/k8flare/k8flare/packages/apiserver-auth"
	kine "github.com/k8flare/k8flare/packages/apiserver-kine"
	flowcontrolv1 "k8s.io/api/flowcontrol/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	fcboot "k8s.io/apiserver/pkg/apis/flowcontrol/bootstrap"
	"k8s.io/apiserver/pkg/authentication/user"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/endpoints/responsewriter"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/apiserver/pkg/util/apihelpers"
	"k8s.io/apiserver/pkg/util/shufflesharding"
	"k8s.io/utils/ptr"
)

const (
	flowControlConfigTTL = 2 * time.Second
	flowControlWaitLimit = 15 * time.Second
)

var flowControlCodec = func() runtime.Codec {
	s := runtime.NewScheme()
	utilruntime.Must(flowcontrolv1.AddToScheme(s))
	return serializer.NewCodecFactory(s).LegacyCodec(flowcontrolv1.SchemeGroupVersion)
}()

type priorityLevel struct {
	config *flowcontrolv1.PriorityLevelConfiguration
	limit  int
	dealer *shufflesharding.Dealer
}

type flowControlConfig struct {
	schemas apihelpers.FlowSchemaSequence
	levels  map[string]*priorityLevel
}

type flowControl struct {
	store                  *kine.Client
	serverConcurrencyLimit int
	now                    func() time.Time

	mu       sync.Mutex
	config   *flowControlConfig
	loadedAt time.Time
}

func newFlowControl(store *kine.Client, serverConcurrencyLimit int) *flowControl {
	return &flowControl{store: store, serverConcurrencyLimit: serverConcurrencyLimit, now: time.Now}
}

func (fc *flowControl) current(ctx context.Context) *flowControlConfig {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if fc.config != nil && fc.now().Sub(fc.loadedAt) < flowControlConfigTTL {
		return fc.config
	}
	levels, schemas, err := fc.load(ctx)
	if err != nil {
		println("apiserver: flow control config:", err.Error())
		if fc.config != nil {
			return fc.config
		}
		return digestFlowControl(nil, nil, fc.serverConcurrencyLimit)
	}
	fc.config = digestFlowControl(levels, schemas, fc.serverConcurrencyLimit)
	fc.loadedAt = fc.now()
	return fc.config
}

func (fc *flowControl) load(ctx context.Context) ([]flowcontrolv1.PriorityLevelConfiguration, []flowcontrolv1.FlowSchema, error) {
	all := storage.ListOptions{Recursive: true, Predicate: storage.Everything}
	levels := &flowcontrolv1.PriorityLevelConfigurationList{}
	if err := kine.NewStorage(fc.store, flowControlCodec, func() runtime.Object { return &flowcontrolv1.PriorityLevelConfiguration{} }).GetList(ctx, "/prioritylevelconfigurations", all, levels); err != nil {
		return nil, nil, err
	}
	schemas := &flowcontrolv1.FlowSchemaList{}
	if err := kine.NewStorage(fc.store, flowControlCodec, func() runtime.Object { return &flowcontrolv1.FlowSchema{} }).GetList(ctx, "/flowschemas", all, schemas); err != nil {
		return nil, nil, err
	}
	return levels.Items, schemas.Items, nil
}

func digestFlowControl(levels []flowcontrolv1.PriorityLevelConfiguration, schemas []flowcontrolv1.FlowSchema, serverConcurrencyLimit int) *flowControlConfig {
	cfg := &flowControlConfig{levels: map[string]*priorityLevel{}}
	for i := range levels {
		if dealer, ok := usablePriorityLevel(&levels[i]); ok {
			cfg.levels[levels[i].Name] = &priorityLevel{config: &levels[i], dealer: dealer}
		}
	}
	var haveExempt, haveCatchAll bool
	for i := range schemas {
		if cfg.levels[schemas[i].Spec.PriorityLevelConfiguration.Name] == nil {
			continue
		}
		cfg.schemas = append(cfg.schemas, &schemas[i])
		haveExempt = haveExempt || schemas[i].Name == flowcontrolv1.FlowSchemaNameExempt
		haveCatchAll = haveCatchAll || schemas[i].Name == flowcontrolv1.FlowSchemaNameCatchAll
	}
	sort.Sort(cfg.schemas)
	if !haveExempt {
		cfg.schemas = append(apihelpers.FlowSchemaSequence{fcboot.MandatoryFlowSchemaExempt}, cfg.schemas...)
	}
	if !haveCatchAll {
		cfg.schemas = append(cfg.schemas, fcboot.MandatoryFlowSchemaCatchAll)
	}
	for _, mandatory := range []*flowcontrolv1.PriorityLevelConfiguration{fcboot.MandatoryPriorityLevelConfigurationExempt, fcboot.MandatoryPriorityLevelConfigurationCatchAll} {
		if cfg.levels[mandatory.Name] == nil {
			dealer, _ := usablePriorityLevel(mandatory)
			cfg.levels[mandatory.Name] = &priorityLevel{config: mandatory, dealer: dealer}
		}
	}
	var shareSum float64
	for _, level := range cfg.levels {
		shareSum += float64(nominalConcurrencyShares(level.config))
	}
	for _, level := range cfg.levels {
		level.limit = int(math.Ceil(float64(serverConcurrencyLimit) * float64(nominalConcurrencyShares(level.config)) / shareSum))
	}
	return cfg
}

func usablePriorityLevel(pl *flowcontrolv1.PriorityLevelConfiguration) (*shufflesharding.Dealer, bool) {
	if (pl.Spec.Type == flowcontrolv1.PriorityLevelEnablementLimited) != (pl.Spec.Limited != nil) ||
		(pl.Spec.Type == flowcontrolv1.PriorityLevelEnablementExempt) != (pl.Spec.Exempt != nil) ||
		(pl.Spec.Type == flowcontrolv1.PriorityLevelEnablementExempt) != (pl.Name == flowcontrolv1.PriorityLevelConfigurationNameExempt) {
		return nil, false
	}
	if pl.Spec.Limited == nil {
		return nil, true
	}
	response := pl.Spec.Limited.LimitResponse
	if (response.Type == flowcontrolv1.LimitResponseTypeReject) != (response.Queuing == nil) {
		return nil, false
	}
	if response.Queuing == nil || response.Queuing.Queues <= 0 {
		return nil, true
	}
	dealer, err := shufflesharding.NewDealer(int(response.Queuing.Queues), int(response.Queuing.HandSize))
	return dealer, err == nil
}

func nominalConcurrencyShares(pl *flowcontrolv1.PriorityLevelConfiguration) int32 {
	if pl.Spec.Limited != nil {
		return ptr.Deref(pl.Spec.Limited.NominalConcurrencyShares, 0)
	}
	return ptr.Deref(pl.Spec.Exempt.NominalConcurrencyShares, 0)
}

func (cfg *flowControlConfig) classify(ri *genericapirequest.RequestInfo, u user.Info) (*flowcontrolv1.FlowSchema, *priorityLevel) {
	var selected, catchAll *flowcontrolv1.FlowSchema
	for _, fs := range cfg.schemas {
		if matchesFlowSchema(ri, u, fs) {
			selected = fs
			break
		}
		if fs.Name == flowcontrolv1.FlowSchemaNameCatchAll {
			catchAll = fs
		}
	}
	if selected == nil {
		selected = catchAll
	}
	return selected, cfg.levels[selected.Spec.PriorityLevelConfiguration.Name]
}

func (l *priorityLevel) seat(fs *flowcontrolv1.FlowSchema, ri *genericapirequest.RequestInfo, u user.Info, rateKey string) kine.SeatRequest {
	seat := kine.SeatRequest{Level: l.config.Name, Limit: l.limit, WaitMs: flowControlWaitLimit.Milliseconds(), User: rateKey}
	queuing := l.config.Spec.Limited.LimitResponse.Queuing
	if queuing == nil || l.dealer == nil {
		return seat
	}
	seat.Queues, seat.QueueLengthLimit = int(queuing.Queues), int(queuing.QueueLengthLimit)
	var hash uint64
	if queuing.Queues > 1 {
		hash = hashFlowID(fs.Name, computeFlowDistinguisher(ri, u, fs.Spec.DistinguisherMethod))
	}
	seat.Hand = l.dealer.DealIntoHand(hash, make([]int, 0, queuing.HandSize))
	return seat
}

func rateLimitKey(r *http.Request, u user.Info) string {
	switch {
	case auth.IsComponentIdentity(u.GetName()):
		return ""
	case u.GetName() == user.Anonymous:
		return "addr:" + r.Header.Get("CF-Connecting-IP")
	}
	return "user:" + u.GetName()
}

func withPriorityAndFairness(next http.Handler, fc *flowControl) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		ri, _ := genericapirequest.RequestInfoFrom(ctx)
		u, _ := genericapirequest.UserFrom(ctx)
		watch := ri.Verb == "watch"
		if longRunningRequest(r, ri) && !watch {
			next.ServeHTTP(w, r)
			return
		}
		fs, level := fc.current(ctx).classify(ri, u)
		w.Header().Set(flowcontrolv1.ResponseHeaderMatchedPriorityLevelConfigurationUID, string(level.config.UID))
		w.Header().Set(flowcontrolv1.ResponseHeaderMatchedFlowSchemaUID, string(fs.UID))
		if level.config.Spec.Type == flowcontrolv1.PriorityLevelEnablementExempt {
			next.ServeHTTP(w, r)
			return
		}
		release, retryAfter, err := fc.store.AcquireSeat(ctx, level.seat(fs, ri, u, rateLimitKey(r, u)))
		if err != nil {
			println("apiserver: flow control seat:", err.Error())
			next.ServeHTTP(w, r)
			return
		}
		if release == nil {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			http.Error(w, "Too many requests, please try again later.", http.StatusTooManyRequests)
			return
		}
		defer release()
		if watch {
			w = responsewriter.WrapForHTTP1Or2(&releaseOnResponse{ResponseWriter: w, release: release})
		}
		next.ServeHTTP(w, r)
	})
}

type releaseOnResponse struct {
	http.ResponseWriter
	release func()
}

func (w *releaseOnResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *releaseOnResponse) WriteHeader(code int) {
	w.release()
	w.ResponseWriter.WriteHeader(code)
}

func (w *releaseOnResponse) Write(b []byte) (int, error) {
	w.release()
	return w.ResponseWriter.Write(b)
}
