
3.0.0

Kubernetes2v1.36.4+k8flare"µª
æ
/apis/autoscaling/v2/§"°
autoscaling_v2get available resources*getAutoscalingV2APIResourcesB◊‘
200Ã
…
OK¬
c
application/jsonO
MK
I#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.APIResourceList
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.APIResourceList
c
application/yamlO
MK
I#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.APIResourceList
¯A
-/apis/autoscaling/v2/horizontalpodautoscalers∆A"‹
autoscaling_v25list or watch objects of kind HorizontalPodAutoscaler*8listAutoscalingV2HorizontalPodAutoscalerForAllNamespacesB‘—
200…
∆
OKø
d
application/jsonP
NL
J#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscalerList
q
application/json;stream=watchP
NL
J#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscalerList
w
#application/vnd.kubernetes.protobufP
NL
J#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscalerList
Ñ
0application/vnd.kubernetes.protobuf;stream=watchP
NL
J#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscalerList
d
application/yamlP
NL
J#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscalerListj
x-kubernetes-actionlist
jb
x-kubernetes-group-version-kind?=group: autoscaling
version: v2
kind: HorizontalPodAutoscaler
j™
ß
allowWatchBookmarksquery˜allowWatchBookmarks requests watch events with type "BOOKMARK". Servers that do not implement bookmarks may ignore this flag and bookmarks are sent at the server's discretion. Clients should not assume bookmarks are returned at any specific interval, nor may they assume the server will send any BOOKMARK event during a session. If this is not a watch, this field is ignored.R
† booleanjÓ	
Î	
continuequery«	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
† stringjá
Ñ
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
† stringjá
Ñ
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
† stringj˘

ˆ

limitquery‘
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
† integerjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† stringj˙
˜
resourceVersionqueryÃresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† stringjŸ
÷
resourceVersionMatchquery¶resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† stringjï
í
sendInitialEventsquery‰
`sendInitialEvents=true` may be set together with `watch=true`. In that case, the watch stream will begin with synthetic events to produce the current state of objects in the collection. Once all such events have been sent, a synthetic "Bookmark" event  will be sent. The bookmark will report the ResourceVersion (RV) corresponding to the set of objects, and be marked with `"k8s.io/initial-events-end": "true"` annotation. Afterwards, the watch stream will proceed as usual, sending watch events corresponding to changes (subsequent to the RV) to objects watched.

When `sendInitialEvents` option is set, we require `resourceVersionMatch` option to also be set. The semantic of the watch request is as following: - `resourceVersionMatch` = NotOlderThan
  is interpreted as "data at least as new as the provided `resourceVersion`"
  and the bookmark event is send when the state is synced
  to a `resourceVersion` at least as fresh as the one provided by the ListOptions.
  If `resourceVersion` is unset, this is interpreted as "consistent read" and the
  bookmark event is send when the state is synced at least to the moment
  when request started being processed.
- `resourceVersionMatch` set to any other value or unset
  Invalid error is returned.

Defaults to true if `resourceVersion=""` or `resourceVersion="0"` (for backward compatibility reasons) and to false otherwise.R
† booleanj¥
±
shardSelectorqueryàshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

  shardRange(object.metadata.uid, '0x0', '0x8000000000000000')
  shardRange(object.metadata.uid, '0x0', '0x8000000000000000') || shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')

Field paths use CEL-style object-rooted syntax (e.g. "object.metadata.uid"), NOT the fieldSelector format ("metadata.uid"). Currently supported paths:
  - object.metadata.uid
  - object.metadata.namespace

hexStart and hexEnd are single-quoted CEL string literals with a '0x' prefix, defining the inclusive lower and exclusive upper bounds over the 64-bit FNV-1a hash space. The full range is [0x0, 0x10000000000000000), where the exclusive upper bound equals 2^64.

Examples:
  2-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x8000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')
  4-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x4000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x4000000000000000', '0x8000000000000000')
    shard 2: shardRange(object.metadata.uid, '0x8000000000000000', '0xc000000000000000')
    shard 3: shardRange(object.metadata.uid, '0xc000000000000000', '0x10000000000000000')

This is an alpha field and requires enabling the ShardedListAndWatch feature gate.R
† stringjù
ö
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
† integerj∞
≠
watchqueryãWatch for changes to the described resources and return them as a stream of add, update, and remove notifications. Specify resourceVersion.R
† boolean
 •
D/apis/autoscaling/v2/namespaces/{namespace}/horizontalpodautoscalersÄ•"ˇ?
autoscaling_v25list or watch objects of kind HorizontalPodAutoscaler*2listAutoscalingV2NamespacedHorizontalPodAutoscaler2™
ß
allowWatchBookmarksquery˜allowWatchBookmarks requests watch events with type "BOOKMARK". Servers that do not implement bookmarks may ignore this flag and bookmarks are sent at the server's discretion. Clients should not assume bookmarks are returned at any specific interval, nor may they assume the server will send any BOOKMARK event during a session. If this is not a watch, this field is ignored.R
† boolean2Ó	
Î	
continuequery«	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
† string2á
Ñ
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
† string2á
Ñ
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
† string2˘

ˆ

limitquery‘
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
† integer2˙
˜
resourceVersionqueryÃresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† string2Ÿ
÷
resourceVersionMatchquery¶resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† string2ï
í
sendInitialEventsquery‰
`sendInitialEvents=true` may be set together with `watch=true`. In that case, the watch stream will begin with synthetic events to produce the current state of objects in the collection. Once all such events have been sent, a synthetic "Bookmark" event  will be sent. The bookmark will report the ResourceVersion (RV) corresponding to the set of objects, and be marked with `"k8s.io/initial-events-end": "true"` annotation. Afterwards, the watch stream will proceed as usual, sending watch events corresponding to changes (subsequent to the RV) to objects watched.

When `sendInitialEvents` option is set, we require `resourceVersionMatch` option to also be set. The semantic of the watch request is as following: - `resourceVersionMatch` = NotOlderThan
  is interpreted as "data at least as new as the provided `resourceVersion`"
  and the bookmark event is send when the state is synced
  to a `resourceVersion` at least as fresh as the one provided by the ListOptions.
  If `resourceVersion` is unset, this is interpreted as "consistent read" and the
  bookmark event is send when the state is synced at least to the moment
  when request started being processed.
- `resourceVersionMatch` set to any other value or unset
  Invalid error is returned.

Defaults to true if `resourceVersion=""` or `resourceVersion="0"` (for backward compatibility reasons) and to false otherwise.R
† boolean2¥
±
shardSelectorqueryàshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

  shardRange(object.metadata.uid, '0x0', '0x8000000000000000')
  shardRange(object.metadata.uid, '0x0', '0x8000000000000000') || shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')

Field paths use CEL-style object-rooted syntax (e.g. "object.metadata.uid"), NOT the fieldSelector format ("metadata.uid"). Currently supported paths:
  - object.metadata.uid
  - object.metadata.namespace

hexStart and hexEnd are single-quoted CEL string literals with a '0x' prefix, defining the inclusive lower and exclusive upper bounds over the 64-bit FNV-1a hash space. The full range is [0x0, 0x10000000000000000), where the exclusive upper bound equals 2^64.

Examples:
  2-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x8000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')
  4-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x4000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x4000000000000000', '0x8000000000000000')
    shard 2: shardRange(object.metadata.uid, '0x8000000000000000', '0xc000000000000000')
    shard 3: shardRange(object.metadata.uid, '0xc000000000000000', '0x10000000000000000')

This is an alpha field and requires enabling the ShardedListAndWatch feature gate.R
† string2ù
ö
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
† integer2∞
≠
watchqueryãWatch for changes to the described resources and return them as a stream of add, update, and remove notifications. Specify resourceVersion.R
† booleanB‘—
200…
∆
OKø
d
application/jsonP
NL
J#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscalerList
q
application/json;stream=watchP
NL
J#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscalerList
w
#application/vnd.kubernetes.protobufP
NL
J#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscalerList
Ñ
0application/vnd.kubernetes.protobuf;stream=watchP
NL
J#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscalerList
d
application/yamlP
NL
J#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscalerListj
x-kubernetes-actionlist
jb
x-kubernetes-group-version-kind?=group: autoscaling
version: v2
kind: HorizontalPodAutoscaler
2◊
autoscaling_v2 create a HorizontalPodAutoscaler*4createAutoscalingV2NamespacedHorizontalPodAutoscaler2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:[
YU
S
*/*L
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscalerBıÀ
200√
¿
OKπ
`
application/jsonL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler
s
#application/vnd.kubernetes.protobufL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler
`
application/yamlL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler–
201»
≈
Createdπ
`
application/jsonL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler
s
#application/vnd.kubernetes.protobufL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler
`
application/yamlL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler—
202…
∆
Acceptedπ
`
application/jsonL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler
s
#application/vnd.kubernetes.protobufL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler
`
application/yamlL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscalerj
x-kubernetes-actionpost
jb
x-kubernetes-group-version-kind?=group: autoscaling
version: v2
kind: HorizontalPodAutoscaler
:ÄL
autoscaling_v2,delete collection of HorizontalPodAutoscaler*>deleteAutoscalingV2CollectionNamespacedHorizontalPodAutoscaler2Ó	
Î	
continuequery«	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
† string2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2á
Ñ
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
† string2„
‡
gracePeriodSecondsquery±The duration in seconds before the object should be deleted. Value must be non-negative integer. The value zero indicates delete immediately. If this value is nil, the default grace period for the specified type will be used. Defaults to a per object value if not specified. zero means delete immediately.R
† integer2®
•
0ignoreStoreReadErrorWithClusterBreakingPotentialqueryÿif set to true, it will trigger an unsafe deletion of the resource in case the normal deletion flow fails with a corrupt object error. A resource is considered corrupt if it can not be retrieved from the underlying storage successfully because of a) its data can not be transformed e.g. decryption failure, or b) it fails to decode into an object. NOTE: unsafe deletion ignores finalizer constraints, skips precondition checks, and removes the object from the storage. WARNING: This may potentially break the cluster if the workload associated with the resource being unsafe-deleted relies on normal deletion flow. Use only if you REALLY know what you are doing. The default value is false, and the user must opt in to enable itR
† boolean2á
Ñ
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
† string2˘

ˆ

limitquery‘
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
† integer2–
Õ
orphanDependentsquery†Deprecated: please use the PropagationPolicy, this field will be deprecated in 1.7. Should the dependent objects be orphaned. If true/false, the "orphan" finalizer will be added to/removed from the object's finalizers list. Either this field or PropagationPolicy may be set, but not both.R
† boolean2á
Ñ
propagationPolicyquery◊Whether and how garbage collection will be performed. Either this field or OrphanDependents may be set, but not both. The default policy is decided by the existing finalizer set in the metadata.finalizers and the resource-specific default policy. Acceptable values are: 'Orphan' - orphan the dependents; 'Background' - allow the garbage collector to delete the dependents in the background; 'Foreground' - a cascading policy that deletes all dependents in the foreground.R
† string2˙
˜
resourceVersionqueryÃresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† string2Ÿ
÷
resourceVersionMatchquery¶resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† string2ï
í
sendInitialEventsquery‰
`sendInitialEvents=true` may be set together with `watch=true`. In that case, the watch stream will begin with synthetic events to produce the current state of objects in the collection. Once all such events have been sent, a synthetic "Bookmark" event  will be sent. The bookmark will report the ResourceVersion (RV) corresponding to the set of objects, and be marked with `"k8s.io/initial-events-end": "true"` annotation. Afterwards, the watch stream will proceed as usual, sending watch events corresponding to changes (subsequent to the RV) to objects watched.

When `sendInitialEvents` option is set, we require `resourceVersionMatch` option to also be set. The semantic of the watch request is as following: - `resourceVersionMatch` = NotOlderThan
  is interpreted as "data at least as new as the provided `resourceVersion`"
  and the bookmark event is send when the state is synced
  to a `resourceVersion` at least as fresh as the one provided by the ListOptions.
  If `resourceVersion` is unset, this is interpreted as "consistent read" and the
  bookmark event is send when the state is synced at least to the moment
  when request started being processed.
- `resourceVersionMatch` set to any other value or unset
  Invalid error is returned.

Defaults to true if `resourceVersion=""` or `resourceVersion="0"` (for backward compatibility reasons) and to false otherwise.R
† boolean2¥
±
shardSelectorqueryàshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

  shardRange(object.metadata.uid, '0x0', '0x8000000000000000')
  shardRange(object.metadata.uid, '0x0', '0x8000000000000000') || shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')

Field paths use CEL-style object-rooted syntax (e.g. "object.metadata.uid"), NOT the fieldSelector format ("metadata.uid"). Currently supported paths:
  - object.metadata.uid
  - object.metadata.namespace

hexStart and hexEnd are single-quoted CEL string literals with a '0x' prefix, defining the inclusive lower and exclusive upper bounds over the 64-bit FNV-1a hash space. The full range is [0x0, 0x10000000000000000), where the exclusive upper bound equals 2^64.

Examples:
  2-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x8000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')
  4-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x4000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x4000000000000000', '0x8000000000000000')
    shard 2: shardRange(object.metadata.uid, '0x8000000000000000', '0xc000000000000000')
    shard 3: shardRange(object.metadata.uid, '0xc000000000000000', '0x10000000000000000')

This is an alpha field and requires enabling the ShardedListAndWatch feature gate.R
† string2ù
ö
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
† integer:Z
XV
T
*/*M
KI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsBºπ
200±
Æ
OKß
Z
application/jsonF
DB
@#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Status
m
#application/vnd.kubernetes.protobufF
DB
@#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Status
Z
application/yamlF
DB
@#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Statusj*
x-kubernetes-actiondeletecollection
jb
x-kubernetes-group-version-kind?=group: autoscaling
version: v2
kind: HorizontalPodAutoscaler
ja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
ÍO
K/apis/autoscaling/v2/namespaces/{namespace}/horizontalpodautoscalers/{name}öO"ƒ
autoscaling_v2*read the specified HorizontalPodAutoscaler*2readAutoscalingV2NamespacedHorizontalPodAutoscalerBŒÀ
200√
¿
OKπ
`
application/jsonL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler
s
#application/vnd.kubernetes.protobufL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler
`
application/yamlL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscalerj
x-kubernetes-actionget
jb
x-kubernetes-group-version-kind?=group: autoscaling
version: v2
kind: HorizontalPodAutoscaler
*ê
autoscaling_v2-replace the specified HorizontalPodAutoscaler*5replaceAutoscalingV2NamespacedHorizontalPodAutoscaler2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:[
YU
S
*/*L
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscalerB°À
200√
¿
OKπ
`
application/jsonL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler
s
#application/vnd.kubernetes.protobufL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler
`
application/yamlL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler–
201»
≈
Createdπ
`
application/jsonL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler
s
#application/vnd.kubernetes.protobufL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler
`
application/yamlL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscalerj
x-kubernetes-actionput
jb
x-kubernetes-group-version-kind?=group: autoscaling
version: v2
kind: HorizontalPodAutoscaler
:˝
autoscaling_v2 delete a HorizontalPodAutoscaler*4deleteAutoscalingV2NamespacedHorizontalPodAutoscaler2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2„
‡
gracePeriodSecondsquery±The duration in seconds before the object should be deleted. Value must be non-negative integer. The value zero indicates delete immediately. If this value is nil, the default grace period for the specified type will be used. Defaults to a per object value if not specified. zero means delete immediately.R
† integer2®
•
0ignoreStoreReadErrorWithClusterBreakingPotentialqueryÿif set to true, it will trigger an unsafe deletion of the resource in case the normal deletion flow fails with a corrupt object error. A resource is considered corrupt if it can not be retrieved from the underlying storage successfully because of a) its data can not be transformed e.g. decryption failure, or b) it fails to decode into an object. NOTE: unsafe deletion ignores finalizer constraints, skips precondition checks, and removes the object from the storage. WARNING: This may potentially break the cluster if the workload associated with the resource being unsafe-deleted relies on normal deletion flow. Use only if you REALLY know what you are doing. The default value is false, and the user must opt in to enable itR
† boolean2–
Õ
orphanDependentsquery†Deprecated: please use the PropagationPolicy, this field will be deprecated in 1.7. Should the dependent objects be orphaned. If true/false, the "orphan" finalizer will be added to/removed from the object's finalizers list. Either this field or PropagationPolicy may be set, but not both.R
† boolean2á
Ñ
propagationPolicyquery◊Whether and how garbage collection will be performed. Either this field or OrphanDependents may be set, but not both. The default policy is decided by the existing finalizer set in the metadata.finalizers and the resource-specific default policy. Acceptable values are: 'Orphan' - orphan the dependents; 'Background' - allow the garbage collector to delete the dependents in the background; 'Foreground' - a cascading policy that deletes all dependents in the foreground.R
† string:Z
XV
T
*/*M
KI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsB¢À
200√
¿
OKπ
`
application/jsonL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler
s
#application/vnd.kubernetes.protobufL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler
`
application/yamlL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler—
202…
∆
Acceptedπ
`
application/jsonL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler
s
#application/vnd.kubernetes.protobufL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler
`
application/yamlL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscalerj 
x-kubernetes-action	delete
jb
x-kubernetes-group-version-kind?=group: autoscaling
version: v2
kind: HorizontalPodAutoscaler
R’
autoscaling_v26partially update the specified HorizontalPodAutoscaler*3patchAutoscalingV2NamespacedHorizontalPodAutoscaler2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2Æ
´
fieldManagerqueryÉfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint. This field is required for apply requests (application/apply-patch) but optional for non-apply patch types (JsonPatch, MergePatch, StrategicMergePatch).R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string2Õ
 
forcequery®Force is going to "force" Apply requests. It means user will re-acquire conflicting fields owned by other people. Force flag must be unset for non-apply patch requests.R
† boolean:≠
™•
e
application/apply-patch+yamlE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Patch
d
application/json-patch+jsonE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Patch
e
application/merge-patch+jsonE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Patch
o
&application/strategic-merge-patch+jsonE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchB°À
200√
¿
OKπ
`
application/jsonL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler
s
#application/vnd.kubernetes.protobufL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler
`
application/yamlL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler–
201»
≈
Createdπ
`
application/jsonL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler
s
#application/vnd.kubernetes.protobufL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler
`
application/yamlL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscalerj
x-kubernetes-actionpatch
jb
x-kubernetes-group-version-kind?=group: autoscaling
version: v2
kind: HorizontalPodAutoscaler
jE
C
namepath#name of the HorizontalPodAutoscaler R
† stringja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
°6
R/apis/autoscaling/v2/namespaces/{namespace}/horizontalpodautoscalers/{name}/status 5"‘
autoscaling_v24read status of the specified HorizontalPodAutoscaler*8readAutoscalingV2NamespacedHorizontalPodAutoscalerStatusBŒÀ
200√
¿
OKπ
`
application/jsonL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler
s
#application/vnd.kubernetes.protobufL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler
`
application/yamlL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscalerj
x-kubernetes-actionget
jb
x-kubernetes-group-version-kind?=group: autoscaling
version: v2
kind: HorizontalPodAutoscaler
*†
autoscaling_v27replace status of the specified HorizontalPodAutoscaler*;replaceAutoscalingV2NamespacedHorizontalPodAutoscalerStatus2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:[
YU
S
*/*L
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscalerB°À
200√
¿
OKπ
`
application/jsonL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler
s
#application/vnd.kubernetes.protobufL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler
`
application/yamlL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler–
201»
≈
Createdπ
`
application/jsonL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler
s
#application/vnd.kubernetes.protobufL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler
`
application/yamlL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscalerj
x-kubernetes-actionput
jb
x-kubernetes-group-version-kind?=group: autoscaling
version: v2
kind: HorizontalPodAutoscaler
RÂ
autoscaling_v2@partially update status of the specified HorizontalPodAutoscaler*9patchAutoscalingV2NamespacedHorizontalPodAutoscalerStatus2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2Æ
´
fieldManagerqueryÉfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint. This field is required for apply requests (application/apply-patch) but optional for non-apply patch types (JsonPatch, MergePatch, StrategicMergePatch).R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string2Õ
 
forcequery®Force is going to "force" Apply requests. It means user will re-acquire conflicting fields owned by other people. Force flag must be unset for non-apply patch requests.R
† boolean:≠
™•
e
application/apply-patch+yamlE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Patch
d
application/json-patch+jsonE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Patch
e
application/merge-patch+jsonE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Patch
o
&application/strategic-merge-patch+jsonE
CA
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchB°À
200√
¿
OKπ
`
application/jsonL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler
s
#application/vnd.kubernetes.protobufL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler
`
application/yamlL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler–
201»
≈
Createdπ
`
application/jsonL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler
s
#application/vnd.kubernetes.protobufL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler
`
application/yamlL
JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscalerj
x-kubernetes-actionpatch
jb
x-kubernetes-group-version-kind?=group: autoscaling
version: v2
kind: HorizontalPodAutoscaler
jE
C
namepath#name of the HorizontalPodAutoscaler R
† stringja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
∏B
3/apis/autoscaling/v2/watch/horizontalpodautoscalersÄB"ñ
autoscaling_v2Éwatch individual changes to a list of HorizontalPodAutoscaler. deprecated: use the 'watch' parameter with a list operation instead.*=watchAutoscalingV2HorizontalPodAutoscalerListForAllNamespacesBµ≤
200™
ß
OK†
^
application/jsonJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
k
application/json;stream=watchJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
q
#application/vnd.kubernetes.protobufJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
~
0application/vnd.kubernetes.protobuf;stream=watchJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
^
application/yamlJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEventj#
x-kubernetes-action
watchlist
jb
x-kubernetes-group-version-kind?=group: autoscaling
version: v2
kind: HorizontalPodAutoscaler
j™
ß
allowWatchBookmarksquery˜allowWatchBookmarks requests watch events with type "BOOKMARK". Servers that do not implement bookmarks may ignore this flag and bookmarks are sent at the server's discretion. Clients should not assume bookmarks are returned at any specific interval, nor may they assume the server will send any BOOKMARK event during a session. If this is not a watch, this field is ignored.R
† booleanjÓ	
Î	
continuequery«	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
† stringjá
Ñ
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
† stringjá
Ñ
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
† stringj˘

ˆ

limitquery‘
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
† integerjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† stringj˙
˜
resourceVersionqueryÃresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† stringjŸ
÷
resourceVersionMatchquery¶resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† stringjï
í
sendInitialEventsquery‰
`sendInitialEvents=true` may be set together with `watch=true`. In that case, the watch stream will begin with synthetic events to produce the current state of objects in the collection. Once all such events have been sent, a synthetic "Bookmark" event  will be sent. The bookmark will report the ResourceVersion (RV) corresponding to the set of objects, and be marked with `"k8s.io/initial-events-end": "true"` annotation. Afterwards, the watch stream will proceed as usual, sending watch events corresponding to changes (subsequent to the RV) to objects watched.

When `sendInitialEvents` option is set, we require `resourceVersionMatch` option to also be set. The semantic of the watch request is as following: - `resourceVersionMatch` = NotOlderThan
  is interpreted as "data at least as new as the provided `resourceVersion`"
  and the bookmark event is send when the state is synced
  to a `resourceVersion` at least as fresh as the one provided by the ListOptions.
  If `resourceVersion` is unset, this is interpreted as "consistent read" and the
  bookmark event is send when the state is synced at least to the moment
  when request started being processed.
- `resourceVersionMatch` set to any other value or unset
  Invalid error is returned.

Defaults to true if `resourceVersion=""` or `resourceVersion="0"` (for backward compatibility reasons) and to false otherwise.R
† booleanj¥
±
shardSelectorqueryàshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

  shardRange(object.metadata.uid, '0x0', '0x8000000000000000')
  shardRange(object.metadata.uid, '0x0', '0x8000000000000000') || shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')

Field paths use CEL-style object-rooted syntax (e.g. "object.metadata.uid"), NOT the fieldSelector format ("metadata.uid"). Currently supported paths:
  - object.metadata.uid
  - object.metadata.namespace

hexStart and hexEnd are single-quoted CEL string literals with a '0x' prefix, defining the inclusive lower and exclusive upper bounds over the 64-bit FNV-1a hash space. The full range is [0x0, 0x10000000000000000), where the exclusive upper bound equals 2^64.

Examples:
  2-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x8000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')
  4-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x4000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x4000000000000000', '0x8000000000000000')
    shard 2: shardRange(object.metadata.uid, '0x8000000000000000', '0xc000000000000000')
    shard 3: shardRange(object.metadata.uid, '0xc000000000000000', '0x10000000000000000')

This is an alpha field and requires enabling the ShardedListAndWatch feature gate.R
† stringjù
ö
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
† integerj∞
≠
watchqueryãWatch for changes to the described resources and return them as a stream of add, update, and remove notifications. Specify resourceVersion.R
† boolean
¨C
J/apis/autoscaling/v2/watch/namespaces/{namespace}/horizontalpodautoscalers›B"ê
autoscaling_v2Éwatch individual changes to a list of HorizontalPodAutoscaler. deprecated: use the 'watch' parameter with a list operation instead.*7watchAutoscalingV2NamespacedHorizontalPodAutoscalerListBµ≤
200™
ß
OK†
^
application/jsonJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
k
application/json;stream=watchJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
q
#application/vnd.kubernetes.protobufJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
~
0application/vnd.kubernetes.protobuf;stream=watchJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
^
application/yamlJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEventj#
x-kubernetes-action
watchlist
jb
x-kubernetes-group-version-kind?=group: autoscaling
version: v2
kind: HorizontalPodAutoscaler
j™
ß
allowWatchBookmarksquery˜allowWatchBookmarks requests watch events with type "BOOKMARK". Servers that do not implement bookmarks may ignore this flag and bookmarks are sent at the server's discretion. Clients should not assume bookmarks are returned at any specific interval, nor may they assume the server will send any BOOKMARK event during a session. If this is not a watch, this field is ignored.R
† booleanjÓ	
Î	
continuequery«	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
† stringjá
Ñ
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
† stringjá
Ñ
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
† stringj˘

ˆ

limitquery‘
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
† integerja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† stringj˙
˜
resourceVersionqueryÃresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† stringjŸ
÷
resourceVersionMatchquery¶resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† stringjï
í
sendInitialEventsquery‰
`sendInitialEvents=true` may be set together with `watch=true`. In that case, the watch stream will begin with synthetic events to produce the current state of objects in the collection. Once all such events have been sent, a synthetic "Bookmark" event  will be sent. The bookmark will report the ResourceVersion (RV) corresponding to the set of objects, and be marked with `"k8s.io/initial-events-end": "true"` annotation. Afterwards, the watch stream will proceed as usual, sending watch events corresponding to changes (subsequent to the RV) to objects watched.

When `sendInitialEvents` option is set, we require `resourceVersionMatch` option to also be set. The semantic of the watch request is as following: - `resourceVersionMatch` = NotOlderThan
  is interpreted as "data at least as new as the provided `resourceVersion`"
  and the bookmark event is send when the state is synced
  to a `resourceVersion` at least as fresh as the one provided by the ListOptions.
  If `resourceVersion` is unset, this is interpreted as "consistent read" and the
  bookmark event is send when the state is synced at least to the moment
  when request started being processed.
- `resourceVersionMatch` set to any other value or unset
  Invalid error is returned.

Defaults to true if `resourceVersion=""` or `resourceVersion="0"` (for backward compatibility reasons) and to false otherwise.R
† booleanj¥
±
shardSelectorqueryàshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

  shardRange(object.metadata.uid, '0x0', '0x8000000000000000')
  shardRange(object.metadata.uid, '0x0', '0x8000000000000000') || shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')

Field paths use CEL-style object-rooted syntax (e.g. "object.metadata.uid"), NOT the fieldSelector format ("metadata.uid"). Currently supported paths:
  - object.metadata.uid
  - object.metadata.namespace

hexStart and hexEnd are single-quoted CEL string literals with a '0x' prefix, defining the inclusive lower and exclusive upper bounds over the 64-bit FNV-1a hash space. The full range is [0x0, 0x10000000000000000), where the exclusive upper bound equals 2^64.

Examples:
  2-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x8000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')
  4-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x4000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x4000000000000000', '0x8000000000000000')
    shard 2: shardRange(object.metadata.uid, '0x8000000000000000', '0xc000000000000000')
    shard 3: shardRange(object.metadata.uid, '0xc000000000000000', '0x10000000000000000')

This is an alpha field and requires enabling the ShardedListAndWatch feature gate.R
† stringjù
ö
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
† integerj∞
≠
watchqueryãWatch for changes to the described resources and return them as a stream of add, update, and remove notifications. Specify resourceVersion.R
† boolean
≠D
Q/apis/autoscaling/v2/watch/namespaces/{namespace}/horizontalpodautoscalers/{name}◊C"√
autoscaling_v2æwatch changes to an object of kind HorizontalPodAutoscaler. deprecated: use the 'watch' parameter with a list operation instead, filtered to a single item with the 'fieldSelector' parameter.*3watchAutoscalingV2NamespacedHorizontalPodAutoscalerBµ≤
200™
ß
OK†
^
application/jsonJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
k
application/json;stream=watchJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
q
#application/vnd.kubernetes.protobufJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
~
0application/vnd.kubernetes.protobuf;stream=watchJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
^
application/yamlJ
HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEventj
x-kubernetes-actionwatch
jb
x-kubernetes-group-version-kind?=group: autoscaling
version: v2
kind: HorizontalPodAutoscaler
j™
ß
allowWatchBookmarksquery˜allowWatchBookmarks requests watch events with type "BOOKMARK". Servers that do not implement bookmarks may ignore this flag and bookmarks are sent at the server's discretion. Clients should not assume bookmarks are returned at any specific interval, nor may they assume the server will send any BOOKMARK event during a session. If this is not a watch, this field is ignored.R
† booleanjÓ	
Î	
continuequery«	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
† stringjá
Ñ
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
† stringjá
Ñ
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
† stringj˘

ˆ

limitquery‘
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
† integerjE
C
namepath#name of the HorizontalPodAutoscaler R
† stringja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† stringj˙
˜
resourceVersionqueryÃresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† stringjŸ
÷
resourceVersionMatchquery¶resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
† stringjï
í
sendInitialEventsquery‰
`sendInitialEvents=true` may be set together with `watch=true`. In that case, the watch stream will begin with synthetic events to produce the current state of objects in the collection. Once all such events have been sent, a synthetic "Bookmark" event  will be sent. The bookmark will report the ResourceVersion (RV) corresponding to the set of objects, and be marked with `"k8s.io/initial-events-end": "true"` annotation. Afterwards, the watch stream will proceed as usual, sending watch events corresponding to changes (subsequent to the RV) to objects watched.

When `sendInitialEvents` option is set, we require `resourceVersionMatch` option to also be set. The semantic of the watch request is as following: - `resourceVersionMatch` = NotOlderThan
  is interpreted as "data at least as new as the provided `resourceVersion`"
  and the bookmark event is send when the state is synced
  to a `resourceVersion` at least as fresh as the one provided by the ListOptions.
  If `resourceVersion` is unset, this is interpreted as "consistent read" and the
  bookmark event is send when the state is synced at least to the moment
  when request started being processed.
- `resourceVersionMatch` set to any other value or unset
  Invalid error is returned.

Defaults to true if `resourceVersion=""` or `resourceVersion="0"` (for backward compatibility reasons) and to false otherwise.R
† booleanj¥
±
shardSelectorqueryàshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

  shardRange(object.metadata.uid, '0x0', '0x8000000000000000')
  shardRange(object.metadata.uid, '0x0', '0x8000000000000000') || shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')

Field paths use CEL-style object-rooted syntax (e.g. "object.metadata.uid"), NOT the fieldSelector format ("metadata.uid"). Currently supported paths:
  - object.metadata.uid
  - object.metadata.namespace

hexStart and hexEnd are single-quoted CEL string literals with a '0x' prefix, defining the inclusive lower and exclusive upper bounds over the 64-bit FNV-1a hash space. The full range is [0x0, 0x10000000000000000), where the exclusive upper bound equals 2^64.

Examples:
  2-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x8000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x8000000000000000', '0x10000000000000000')
  4-shard split:
    shard 0: shardRange(object.metadata.uid, '0x0000000000000000', '0x4000000000000000')
    shard 1: shardRange(object.metadata.uid, '0x4000000000000000', '0x8000000000000000')
    shard 2: shardRange(object.metadata.uid, '0x8000000000000000', '0xc000000000000000')
    shard 3: shardRange(object.metadata.uid, '0xc000000000000000', '0x10000000000000000')

This is an alpha field and requires enabling the ShardedListAndWatch feature gate.R
† stringjù
ö
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
† integerj∞
≠
watchqueryãWatch for changes to the described resources and return them as a stream of add, update, and remove notifications. Specify resourceVersion.R
† boolean*¸n
˘n
Ù
7io.k8s.api.autoscaling.v2.ContainerResourceMetricSource∏
µ∫name∫target∫	container object˙å

	container
 stringä 

name
 stringä 
Q
targetG
E“?=
;#/components/schemas/io.k8s.api.autoscaling.v2.MetricTargetä 
˚
7io.k8s.api.autoscaling.v2.ContainerResourceMetricStatusø
º∫name∫current∫	container object˙í

	container
 stringä 
W
currentL
J“DB
@#/components/schemas/io.k8s.api.autoscaling.v2.MetricValueStatusä 

name
 stringä 
§
5io.k8s.api.autoscaling.v2.CrossVersionObjectReferencek
i∫kind∫name object˙O


apiVersion
	 string

kind
 stringä 

name
 stringä 
ˇ
.io.k8s.api.autoscaling.v2.ExternalMetricSourceÃ
…∫metric∫target object˙™
U
metricK
I“CA
?#/components/schemas/io.k8s.api.autoscaling.v2.MetricIdentifierä 
Q
targetG
E“?=
;#/components/schemas/io.k8s.api.autoscaling.v2.MetricTargetä 
Ü
.io.k8s.api.autoscaling.v2.ExternalMetricStatus”
–∫metric∫current object˙∞
W
currentL
J“DB
@#/components/schemas/io.k8s.api.autoscaling.v2.MetricValueStatusä 
U
metricK
I“CA
?#/components/schemas/io.k8s.api.autoscaling.v2.MetricIdentifierä 
’
*io.k8s.api.autoscaling.v2.HPAScalingPolicy¶
£∫type∫value∫periodSeconds object˙x
1
periodSeconds 
 integerä		        öint32

type
 stringä 
)
value 
 integerä		        öint32
Ù
)io.k8s.api.autoscaling.v2.HPAScalingRules∆
√ object˙∂
ç
policiesÄ
~ arrayÚM
K
I“CA
?#/components/schemas/io.k8s.api.autoscaling.v2.HPAScalingPolicyä ¢#
x-kubernetes-list-type	atomic


selectPolicy
	 string
2
stabilizationWindowSeconds
 integeröint32
S
	toleranceFD
B#/components/schemas/io.k8s.apimachinery.pkg.api.resource.Quantity
ä
1io.k8s.api.autoscaling.v2.HorizontalPodAutoscaler‘
—∫spec object˙“


apiVersion
	 string

kind
	 string
\
metadataP
N“HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMetaä 
^
specV
T“NL
J#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscalerSpecä 
b
statusX
V“PN
L#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscalerStatusä ¢h
x-kubernetes-group-version-kindEC- group: autoscaling
  kind: HorizontalPodAutoscaler
  version: v2

Ó
9io.k8s.api.autoscaling.v2.HorizontalPodAutoscalerBehavior∞
≠ object˙†
O
	scaleDownB@
>#/components/schemas/io.k8s.api.autoscaling.v2.HPAScalingRules
M
scaleUpB@
>#/components/schemas/io.k8s.api.autoscaling.v2.HPAScalingRules
û
:io.k8s.api.autoscaling.v2.HorizontalPodAutoscalerConditionﬂ
‹∫type∫status object˙ø
X
lastTransitionTimeB@
>#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Time

message
	 string

reason
	 string

status
 stringä 

type
 stringä 
π
5io.k8s.api.autoscaling.v2.HorizontalPodAutoscalerListˇ
¸∫items object˙¯


apiVersion
	 string
j
itemsa
_ arrayÚT
R
P“JH
F#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscalerä 

kind
	 string
Z
metadataN
L“FD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaä ¢l
x-kubernetes-group-version-kindIG- group: autoscaling
  kind: HorizontalPodAutoscalerList
  version: v2

ë
5io.k8s.api.autoscaling.v2.HorizontalPodAutoscalerSpec◊
‘∫scaleTargetRef∫maxReplicas object˙®
^
behaviorRP
N#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscalerBehavior
/
maxReplicas 
 integerä		        öint32
Ö
metricsz
x arrayÚG
E
C“=;
9#/components/schemas/io.k8s.api.autoscaling.v2.MetricSpecä ¢#
x-kubernetes-list-type	atomic

#
minReplicas
 integeröint32
h
scaleTargetRefV
T“NL
J#/components/schemas/io.k8s.api.autoscaling.v2.CrossVersionObjectReferenceä 
Ï
7io.k8s.api.autoscaling.v2.HorizontalPodAutoscalerStatus∞
≠∫desiredReplicas object˙é
õ

conditionså
â arrayÚ]
[
Y“SQ
O#/components/schemas/io.k8s.api.autoscaling.v2.HorizontalPodAutoscalerConditionä ¢'
x-kubernetes-list-map-keys	- type
¢ 
x-kubernetes-list-typemap
¢'
x-kubernetes-patch-merge-keytype
¢'
x-kubernetes-patch-strategymerge

é
currentMetrics|
z arrayÚI
G
E“?=
;#/components/schemas/io.k8s.api.autoscaling.v2.MetricStatusä ¢#
x-kubernetes-list-type	atomic

'
currentReplicas
 integeröint32
3
desiredReplicas 
 integerä		        öint32
S
lastScaleTimeB@
>#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Time
*
observedGeneration
 integeröint64
∏
*io.k8s.api.autoscaling.v2.MetricIdentifierâ
Ü∫name object˙s

name
 stringä 
W
selectorKI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.LabelSelector
â
$io.k8s.api.autoscaling.v2.MetricSpec‡
›∫type object˙…
e
containerResourcePN
L#/components/schemas/io.k8s.api.autoscaling.v2.ContainerResourceMetricSource
S
externalGE
C#/components/schemas/io.k8s.api.autoscaling.v2.ExternalMetricSource
O
objectEC
A#/components/schemas/io.k8s.api.autoscaling.v2.ObjectMetricSource
K
podsCA
?#/components/schemas/io.k8s.api.autoscaling.v2.PodsMetricSource
S
resourceGE
C#/components/schemas/io.k8s.api.autoscaling.v2.ResourceMetricSource

type
 stringä 
ã
&io.k8s.api.autoscaling.v2.MetricStatus‡
›∫type object˙…
e
containerResourcePN
L#/components/schemas/io.k8s.api.autoscaling.v2.ContainerResourceMetricStatus
S
externalGE
C#/components/schemas/io.k8s.api.autoscaling.v2.ExternalMetricStatus
O
objectEC
A#/components/schemas/io.k8s.api.autoscaling.v2.ObjectMetricStatus
K
podsCA
?#/components/schemas/io.k8s.api.autoscaling.v2.PodsMetricStatus
S
resourceGE
C#/components/schemas/io.k8s.api.autoscaling.v2.ResourceMetricStatus

type
 stringä 
±
&io.k8s.api.autoscaling.v2.MetricTargetÜ
É∫type object˙Ô
*
averageUtilization
 integeröint32
V
averageValueFD
B#/components/schemas/io.k8s.apimachinery.pkg.api.resource.Quantity

type
 stringä 
O
valueFD
B#/components/schemas/io.k8s.apimachinery.pkg.api.resource.Quantity
ï
+io.k8s.api.autoscaling.v2.MetricValueStatusÂ
‚ object˙’
*
averageUtilization
 integeröint32
V
averageValueFD
B#/components/schemas/io.k8s.apimachinery.pkg.api.resource.Quantity
O
valueFD
B#/components/schemas/io.k8s.apimachinery.pkg.api.resource.Quantity
˙
,io.k8s.api.autoscaling.v2.ObjectMetricSource…
∆∫describedObject∫target∫metric object˙ï
i
describedObjectV
T“NL
J#/components/schemas/io.k8s.api.autoscaling.v2.CrossVersionObjectReferenceä 
U
metricK
I“CA
?#/components/schemas/io.k8s.api.autoscaling.v2.MetricIdentifierä 
Q
targetG
E“?=
;#/components/schemas/io.k8s.api.autoscaling.v2.MetricTargetä 
Å
,io.k8s.api.autoscaling.v2.ObjectMetricStatus–
Õ∫metric∫current∫describedObject object˙õ
W
currentL
J“DB
@#/components/schemas/io.k8s.api.autoscaling.v2.MetricValueStatusä 
i
describedObjectV
T“NL
J#/components/schemas/io.k8s.api.autoscaling.v2.CrossVersionObjectReferenceä 
U
metricK
I“CA
?#/components/schemas/io.k8s.api.autoscaling.v2.MetricIdentifierä 
˚
*io.k8s.api.autoscaling.v2.PodsMetricSourceÃ
…∫metric∫target object˙™
U
metricK
I“CA
?#/components/schemas/io.k8s.api.autoscaling.v2.MetricIdentifierä 
Q
targetG
E“?=
;#/components/schemas/io.k8s.api.autoscaling.v2.MetricTargetä 
Ç
*io.k8s.api.autoscaling.v2.PodsMetricStatus”
–∫metric∫current object˙∞
W
currentL
J“DB
@#/components/schemas/io.k8s.api.autoscaling.v2.MetricValueStatusä 
U
metricK
I“CA
?#/components/schemas/io.k8s.api.autoscaling.v2.MetricIdentifierä 
ø
.io.k8s.api.autoscaling.v2.ResourceMetricSourceå
â∫name∫target object˙m

name
 stringä 
Q
targetG
E“?=
;#/components/schemas/io.k8s.api.autoscaling.v2.MetricTargetä 
∆
.io.k8s.api.autoscaling.v2.ResourceMetricStatusì
ê∫name∫current object˙s
W
currentL
J“DB
@#/components/schemas/io.k8s.api.autoscaling.v2.MetricValueStatusä 

name
 stringä 
O
-io.k8s.apimachinery.pkg.api.resource.Quantity
⁄
	 string⁄
	 number
ì
0io.k8s.apimachinery.pkg.apis.meta.v1.APIResourceﬁ
€∫name∫singularName∫
namespaced∫kind∫verbs object˙ú
S

categoriesE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic


group
	 string

kind
 stringä 

name
 stringä 


namespaced
 booleanä 
S

shortNamesE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

 
singularName
 stringä 
!
storageVersionHash
	 string
(
verbs
 arrayÚ

 stringä 

version
	 string
®
4io.k8s.apimachinery.pkg.apis.meta.v1.APIResourceListÔ
Ï∫groupVersion∫	resources object˙Í


apiVersion
	 string
 
groupVersion
 stringä 

kind
	 string
ï
	resourcesá
Ñ arrayÚS
Q
O“IG
E#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.APIResourceä ¢#
x-kubernetes-list-type	atomic
¢W
x-kubernetes-group-version-kind42- group: ""
  kind: APIResourceList
  version: v1

Í
2io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptions≥
∞ object˙ë


apiVersion
	 string
O
dryRunE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

*
gracePeriodSeconds
 integeröint64
@
0ignoreStoreReadErrorWithClusterBreakingPotential

 boolean

kind
	 string
 
orphanDependents

 boolean
\
preconditionsKI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Preconditions
 
propagationPolicy
	 string¢é
x-kubernetes-group-version-kindki- group: ""
  kind: DeleteOptions
  version: v1
- group: autoscaling
  kind: DeleteOptions
  version: v2

<
-io.k8s.apimachinery.pkg.apis.meta.v1.FieldsV1
	 object
…
2io.k8s.apimachinery.pkg.apis.meta.v1.LabelSelectorí
è object˙›
©
matchExpressionsî
ë arrayÚ`
^
\“VT
R#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.LabelSelectorRequirementä ¢#
x-kubernetes-list-type	atomic

/
matchLabels 
 objectÇ

 stringä ¢"
x-kubernetes-map-type	atomic

Î
=io.k8s.apimachinery.pkg.apis.meta.v1.LabelSelectorRequirement©
¶∫key∫operator object˙à

key
 stringä 

operator
 stringä 
O
valuesE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

ñ
-io.k8s.apimachinery.pkg.apis.meta.v1.ListMeta‰
· object˙‘

continue
	 string
*
remainingItemCount
 integeröint64

resourceVersion
	 string

selfLink
	 string
T
	shardInfoGE
C#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ShardInfo

7io.k8s.apimachinery.pkg.apis.meta.v1.ManagedFieldsEntry¥
± object˙§


apiVersion
	 string


fieldsType
	 string
R
fieldsV1FD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.FieldsV1

manager
	 string

	operation
	 string

subresource
	 string
J
timeB@
>#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Time
˝
/io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMeta…
∆ object˙π
/
annotations 
 objectÇ

 stringä 
W
creationTimestampB@
>#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Time
2
deletionGracePeriodSeconds
 integeröint64
W
deletionTimestampB@
>#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Time
z

finalizersl
j arrayÚ

 stringä ¢ 
x-kubernetes-list-typeset
¢'
x-kubernetes-patch-strategymerge


generateName
	 string
"

generation
 integeröint64
*
labels 
 objectÇ

 stringä 
†
managedFieldsé
ã arrayÚZ
X
V“PN
L#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ManagedFieldsEntryä ¢#
x-kubernetes-list-type	atomic


name
	 string

	namespace
	 string
ó
ownerReferencesÉ
Ä arrayÚV
T
R“LJ
H#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.OwnerReferenceä ¢&
x-kubernetes-list-map-keys- uid
¢ 
x-kubernetes-list-typemap
¢&
x-kubernetes-patch-merge-keyuid
¢'
x-kubernetes-patch-strategymerge


resourceVersion
	 string

selfLink
	 string

uid
	 string
ª
3io.k8s.apimachinery.pkg.apis.meta.v1.OwnerReferenceÉ
Ä∫
apiVersion∫kind∫name∫uid object˙≠


apiVersion
 stringä 
"
blockOwnerDeletion

 boolean


controller

 boolean

kind
 stringä 

name
 stringä 

uid
 stringä ¢"
x-kubernetes-map-type	atomic

9
*io.k8s.apimachinery.pkg.apis.meta.v1.Patch
	 object
x
2io.k8s.apimachinery.pkg.apis.meta.v1.PreconditionsB
@ object˙4

resourceVersion
	 string

uid
	 string
i
.io.k8s.apimachinery.pkg.apis.meta.v1.ShardInfo7
5∫selector object˙

selector
 stringä 
Ÿ
+io.k8s.apimachinery.pkg.apis.meta.v1.Status©
¶ object˙»


apiVersion
	 string

code
 integeröint32
V
detailsKI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.StatusDetails

kind
	 string

message
	 string
Z
metadataN
L“FD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaä 

reason
	 string

status
	 string¢N
x-kubernetes-group-version-kind+)- group: ""
  kind: Status
  version: v1

á
0io.k8s.apimachinery.pkg.apis.meta.v1.StatusCauseS
Q object˙E

field
	 string

message
	 string

reason
	 string
€
2io.k8s.apimachinery.pkg.apis.meta.v1.StatusDetails§
° object˙î
í
causesá
Ñ arrayÚS
Q
O“IG
E#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.StatusCauseä ¢#
x-kubernetes-list-type	atomic


group
	 string

kind
	 string

name
	 string
)
retryAfterSeconds
 integeröint32

uid
	 string
D
)io.k8s.apimachinery.pkg.apis.meta.v1.Time
 stringö	date-time
 
/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEventñ
ì∫type∫object object˙k
O
objectEC
A#/components/schemas/io.k8s.apimachinery.pkg.runtime.RawExtension

type
 stringä ¢à
x-kubernetes-group-version-kindec- group: ""
  kind: WatchEvent
  version: v1
- group: autoscaling
  kind: WatchEvent
  version: v2

;
,io.k8s.apimachinery.pkg.runtime.RawExtension
	 object