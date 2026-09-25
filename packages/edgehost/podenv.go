package edgehost

import (
	"encoding/json"
	"net/http"
	"sort"
)

func MergePodEnvHandler(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Pod  json.RawMessage   `json:"pod"`
		Vars map[string]string `json:"vars"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	out, err := MergePodEnv(in.Pod, in.Vars)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(out)
}

func MergePodEnv(pod []byte, vars map[string]string) ([]byte, error) {
	if len(vars) == 0 {
		return pod, nil
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(pod, &doc); err != nil {
		return nil, err
	}
	var spec map[string]json.RawMessage
	if err := json.Unmarshal(doc["spec"], &spec); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(vars))
	for name := range vars {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, key := range []string{"containers", "initContainers"} {
		if len(spec[key]) == 0 {
			continue
		}
		var containers []map[string]json.RawMessage
		if err := json.Unmarshal(spec[key], &containers); err != nil {
			return nil, err
		}
		for i := range containers {
			var env []podEnv
			if len(containers[i]["env"]) > 0 {
				if err := json.Unmarshal(containers[i]["env"], &env); err != nil {
					return nil, err
				}
			}
			have := map[string]bool{}
			for _, e := range env {
				have[e.Name] = true
			}
			for _, name := range names {
				if !have[name] {
					env = append(env, podEnv{Name: name, Value: vars[name]})
				}
			}
			raw, err := json.Marshal(env)
			if err != nil {
				return nil, err
			}
			containers[i]["env"] = raw
		}
		raw, err := json.Marshal(containers)
		if err != nil {
			return nil, err
		}
		spec[key] = raw
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}
	doc["spec"] = raw
	return json.Marshal(doc)
}

type podEnv struct {
	Name  string `json:"name"`
	Value string `json:"value,omitempty"`
}
