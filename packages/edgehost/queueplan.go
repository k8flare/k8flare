package edgehost

import (
	"encoding/json"
	"net/http"
	"strings"
)

type QueuePlan struct {
	Resources   []string `json:"resources"`
	ServiceKeys []string `json:"serviceKeys"`
	GatewayKeys []string `json:"gatewayKeys"`
}

type queueMessage struct {
	Kind    string   `json:"kind"`
	Key     string   `json:"key"`
	Changed []string `json:"changed"`
}

func PlanQueue(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Messages []queueMessage `json:"messages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(planQueue(body.Messages))
}

func planQueue(msgs []queueMessage) QueuePlan {
	resources := map[string]bool{}
	var order []string
	add := func(name string) {
		if name == "" || resources[name] {
			return
		}
		resources[name] = true
		order = append(order, name)
	}
	var plan QueuePlan
	for _, msg := range msgs {
		if msg.Kind == "change" {
			parts := strings.Split(msg.Key, "/")
			if len(parts) > 2 {
				add(parts[2])
				if strings.Contains(parts[2], ".") && len(parts) > 3 {
					add(parts[3])
				}
			}
			if strings.HasPrefix(msg.Key, "/registry/services/") {
				plan.ServiceKeys = append(plan.ServiceKeys, msg.Key)
			}
			if strings.HasPrefix(msg.Key, gatewayAPIPrefix) || strings.HasPrefix(msg.Key, ingressPrefix) || strings.HasPrefix(msg.Key, ingressClassPrefix) || strings.HasPrefix(msg.Key, servicePrefix) {
				plan.GatewayKeys = append(plan.GatewayKeys, msg.Key)
			}
		} else if msg.Kind == "retry" {
			for _, name := range msg.Changed {
				add(name)
			}
		}
	}
	plan.Resources = order
	if plan.Resources == nil {
		plan.Resources = []string{}
	}
	if plan.ServiceKeys == nil {
		plan.ServiceKeys = []string{}
	}
	if plan.GatewayKeys == nil {
		plan.GatewayKeys = []string{}
	}
	return plan
}
