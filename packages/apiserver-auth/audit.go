package auth

import (
	"context"
	"encoding/base64"
	"net/http"

	auditinternal "k8s.io/apiserver/pkg/apis/audit"
	"k8s.io/apiserver/pkg/audit"
	"k8s.io/apiserver/pkg/endpoints/responsewriter"
)

const (
	auditLevelHeader             = "X-K8flare-Audit-Level"
	auditOmitManagedFieldsHeader = "X-K8flare-Audit-Omit-Managed-Fields"
	auditRequestObjectHeader     = "X-K8flare-Audit-Request-Object"
	auditResponseObjectHeader    = "X-K8flare-Audit-Response-Object"
)

func ForwardAuditConfig(h http.Header, rac audit.RequestAuditConfig) {
	h.Del(auditLevelHeader)
	h.Del(auditOmitManagedFieldsHeader)
	if rac.Level.Less(auditinternal.LevelRequest) {
		return
	}
	h.Set(auditLevelHeader, string(rac.Level))
	if rac.OmitManagedFields {
		h.Set(auditOmitManagedFieldsHeader, "true")
	}
}

func TakeForwardedAuditObjects(h http.Header) (request, response []byte) {
	request, _ = base64.StdEncoding.DecodeString(h.Get(auditRequestObjectHeader))
	response, _ = base64.StdEncoding.DecodeString(h.Get(auditResponseObjectHeader))
	h.Del(auditRequestObjectHeader)
	h.Del(auditResponseObjectHeader)
	return request, response
}

func WithForwardedAudit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rac := audit.RequestAuditConfig{
			Level:             auditinternal.Level(r.Header.Get(auditLevelHeader)),
			OmitManagedFields: r.Header.Get(auditOmitManagedFieldsHeader) == "true",
		}
		if rac.Level.Less(auditinternal.LevelRequest) {
			next.ServeHTTP(w, r)
			return
		}
		ctx := audit.WithAuditContext(r.Context())
		objects := &auditObjectsWriter{ResponseWriter: w, ctx: ctx}
		_ = audit.AuditContextFrom(ctx).Init(rac, objects)
		next.ServeHTTP(responsewriter.WrapForHTTP1Or2(objects), r.WithContext(ctx))
	})
}

type auditObjectsWriter struct {
	http.ResponseWriter
	ctx     context.Context
	started bool
}

func (w *auditObjectsWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *auditObjectsWriter) WriteHeader(code int) {
	w.start()
	w.ResponseWriter.WriteHeader(code)
}

func (w *auditObjectsWriter) Write(b []byte) (int, error) {
	w.start()
	return w.ResponseWriter.Write(b)
}

func (w *auditObjectsWriter) start() {
	if w.started {
		return
	}
	w.started = true
	audit.AuditContextFrom(w.ctx).ProcessEventStage(w.ctx, auditinternal.StageResponseStarted)
}

func (w *auditObjectsWriter) ProcessEvents(events ...*auditinternal.Event) bool {
	for _, ev := range events {
		if ev.RequestObject != nil {
			w.Header().Set(auditRequestObjectHeader, base64.StdEncoding.EncodeToString(ev.RequestObject.Raw))
		}
		if ev.ResponseObject != nil {
			w.Header().Set(auditResponseObjectHeader, base64.StdEncoding.EncodeToString(ev.ResponseObject.Raw))
		}
	}
	return true
}
