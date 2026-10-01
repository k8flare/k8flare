import { bindings, defineConfig, exports, triggers } from "cf/config";

export default defineConfig({
	worker: {
		name: "k8flare",
		compatibilityDate: "2026-09-01",
		compatibilityFlags: [
			"new_module_registry",
		],
		entrypoint: "src/index.ts",
		workersDev: true,
		observability: {
			enabled: true,
			logs: {
				enabled: true,
				headSamplingRate: 1,
				persist: true,
			},
			traces: {
				enabled: true,
				headSamplingRate: 0.1,
				persist: true,
			},
		},
		assets: {
			runWorkerFirst: [
				"/wasm/*",
			],
		},
		domains: [
			"k8flare.kooffice.jp",
		],
		triggers: [
			triggers.queue({
				deadLetterQueue: "k8flare-dlq",
				maxBatchSize: 100,
				maxBatchTimeout: 1,
				maxConcurrency: 1,
				maxRetries: 10,
				name: "k8flare-controllers",
			}),
			triggers.queue({
				deadLetterQueue: "k8flare-dlq",
				maxBatchSize: 100,
				maxBatchTimeout: 1,
				maxConcurrency: 1,
				maxRetries: 10,
				name: "k8flare-scheduler",
			}),
			triggers.queue({
				deadLetterQueue: "k8flare-dlq",
				maxBatchSize: 100,
				maxBatchTimeout: 1,
				maxConcurrency: 1,
				maxRetries: 10,
				name: "k8flare-workloads",
			}),
			triggers.queue({
				deadLetterQueue: "k8flare-dlq",
				maxBatchSize: 100,
				maxBatchTimeout: 1,
				maxConcurrency: 1,
				maxRetries: 10,
				name: "k8flare-crds",
			}),
			triggers.queue({
				deadLetterQueue: "k8flare-dlq",
				maxBatchSize: 100,
				maxBatchTimeout: 1,
				maxConcurrency: 1,
				maxRetries: 10,
				name: "k8flare-gc",
			}),
			triggers.queue({
				deadLetterQueue: "k8flare-dlq",
				maxBatchSize: 100,
				maxBatchTimeout: 1,
				maxConcurrency: 1,
				maxRetries: 10,
				name: "k8flare-accounts",
			}),
			triggers.queue({
				deadLetterQueue: "k8flare-dlq",
				maxBatchSize: 100,
				maxBatchTimeout: 1,
				maxConcurrency: 1,
				maxRetries: 10,
				name: "k8flare-extensions",
			}),
			triggers.queue({
				deadLetterQueue: "k8flare-dlq",
				maxBatchSize: 100,
				maxBatchTimeout: 1,
				maxConcurrency: 1,
				maxRetries: 10,
				name: "k8flare-metrics",
			}),
			triggers.queue({
				deadLetterQueue: "k8flare-dlq",
				maxBatchSize: 100,
				maxBatchTimeout: 1,
				maxConcurrency: 1,
				maxRetries: 10,
				name: "k8flare-containers",
			}),
			triggers.queue({
				deadLetterQueue: "k8flare-dlq",
				maxBatchSize: 100,
				maxBatchTimeout: 1,
				maxConcurrency: 1,
				maxRetries: 10,
				name: "k8flare-attachdetach",
			}),
			triggers.queue({
				deadLetterQueue: "k8flare-dlq",
				maxBatchSize: 100,
				maxBatchTimeout: 1,
				maxConcurrency: 1,
				maxRetries: 10,
				name: "k8flare-addons",
			}),
		],
		env: {
			CLUSTER_DOMAIN: bindings.text("k8flare.com"),
			API_HOSTS: bindings.text("k8flare.kooffice.jp"),
			ACCESS_TEAM_DOMAIN: bindings.text("kooffice"),
			ACCESS_AUD: bindings.text("f5b5bb1f97c02d095ecd20352a1e2282ddef9fca76a977800783a80172cc511e"),
			ACCESS_GROUPS_CLAIM: bindings.text("groups"),
			ACCESS_GROUPS_PREFIX: bindings.text("access:"),
			OIDC_ISSUER_URL: bindings.text(""),
			OIDC_CLIENT_ID: bindings.text(""),
			OIDC_USERNAME_CLAIM: bindings.text(""),
			OIDC_USERNAME_PREFIX: bindings.text(""),
			OIDC_GROUPS_CLAIM: bindings.text(""),
			OIDC_GROUPS_PREFIX: bindings.text(""),
			OIDC_REQUIRED_CLAIMS: bindings.text(""),
			MAX_REQUESTS_INFLIGHT: bindings.text("400"),
			MAX_MUTATING_REQUESTS_INFLIGHT: bindings.text("200"),
			CLUSTER_UID: bindings.text(""),
			AUDIT_POLICY: bindings.text(""),
			DISABLE: bindings.text(""),
			GATEWAY_URL: bindings.text("https://api.k8flare.com"),
			R2_ACCOUNT_ID: bindings.text("ed17c5c18eb6052e70234ec181709fba"),
			R2_BUCKET: bindings.text("k8flare-pods"),
			CLOUDFLARE_ACCOUNT_ID: bindings.text("ed17c5c18eb6052e70234ec181709fba"),
			PODS_R2: bindings.r2({
				name: "k8flare-pods",
			}),
			MANIFESTS_R2: bindings.r2({
				name: "k8flare-manifests",
			}),
			CTRL_Q: bindings.queue({
				name: "k8flare-controllers",
			}),
			SCHED_Q: bindings.queue({
				name: "k8flare-scheduler",
			}),
			WL_Q: bindings.queue({
				name: "k8flare-workloads",
			}),
			CRD_Q: bindings.queue({
				name: "k8flare-crds",
			}),
			GC_Q: bindings.queue({
				name: "k8flare-gc",
			}),
			ACCT_Q: bindings.queue({
				name: "k8flare-accounts",
			}),
			EXT_Q: bindings.queue({
				name: "k8flare-extensions",
			}),
			METRICS_Q: bindings.queue({
				name: "k8flare-metrics",
			}),
			CONTAINERS_Q: bindings.queue({
				name: "k8flare-containers",
			}),
			AD_Q: bindings.queue({
				name: "k8flare-attachdetach",
			}),
			ADDON_Q: bindings.queue({
				name: "k8flare-addons",
			}),
			HPA_Q: bindings.queue({
				name: "k8flare-hpa",
			}),
			PRINTERS: bindings.worker({
				worker: "k8flare",
				exportName: "Printers",
			}),
			OPENAPI: bindings.worker({
				worker: "k8flare",
				exportName: "OpenAPI",
			}),
			CUSTOMRESOURCES: bindings.worker({
				worker: "k8flare",
				exportName: "CustomResources",
			}),
			APIGROUPS: bindings.worker({
				worker: "k8flare",
				exportName: "APIGroups",
			}),
			SCHEDULER: bindings.worker({
				worker: "k8flare",
				exportName: "Scheduler",
			}),
			WORKLOADS: bindings.worker({
				worker: "k8flare",
				exportName: "Workloads",
			}),
			ATTACHDETACH: bindings.worker({
				worker: "k8flare",
				exportName: "AttachDetach",
			}),
			GC: bindings.worker({
				worker: "k8flare",
				exportName: "GarbageCollector",
			}),
			STORAGE_SVC: bindings.worker({
				worker: "k8flare",
				exportName: "Storage",
			}),
			TUNNEL: bindings.worker({
				worker: "k8flare",
				exportName: "NodeTunnels",
			}),
			ADMISSION: bindings.worker({
				worker: "k8flare",
				exportName: "Admission",
			}),
			HOOKS: bindings.worker({
				worker: "k8flare",
				exportName: "Hooks",
			}),
			OUTBOUND: bindings.worker({
				worker: "k8flare",
				exportName: "Outbound",
			}),
			METRICS: bindings.worker({
				worker: "k8flare",
				exportName: "Metrics",
			}),
			APISERVER: bindings.worker({
				worker: "k8flare",
			}),
			CLUSTER: bindings.durableObject({
				worker: "k8flare",
				exportName: "Cluster",
			}),
			NODE_TUNNEL: bindings.durableObject({
				worker: "k8flare",
				exportName: "NodeTunnel",
			}),
			NODE_SCHED: bindings.durableObject({
				worker: "k8flare",
				exportName: "CFContainersScheduler",
			}),
			NODE_VM_SMALL: bindings.durableObject({
				worker: "k8flare",
				exportName: "NodeVMSmall",
			}),
			NODE_VM_MEDIUM: bindings.durableObject({
				worker: "k8flare",
				exportName: "NodeVMMedium",
			}),
			NODE_VM_LARGE: bindings.durableObject({
				worker: "k8flare",
				exportName: "NodeVMLarge",
			}),
			POD_KUBELET: bindings.durableObject({
				worker: "k8flare",
				exportName: "PodKubelet",
			}),
			POD_LEDGER: bindings.durableObject({
				worker: "k8flare",
				exportName: "PodLedger",
			}),
			CF_VERSION: bindings.versionMetadata(),
			LOADER: bindings.workerLoader(),
			ASSETS: bindings.assets(),
		},
		exports: {
			Cluster: exports.durableObject({ storage: "sqlite" }),
			NodeTunnel: exports.durableObject({ storage: "sqlite" }),
			CFContainersScheduler: exports.durableObject({ storage: "sqlite" }),
			NodeVMSmall: exports.durableObject({ storage: "sqlite" }),
			NodeVMMedium: exports.durableObject({ storage: "sqlite" }),
			NodeVMLarge: exports.durableObject({ storage: "sqlite" }),
			PodKubelet: exports.durableObject({ storage: "sqlite" }),
			PodLedger: exports.durableObject({ storage: "sqlite" }),
		},
	},
});
