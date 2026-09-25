
3.0.0

Kubernetes2v1.36.4+k8flare"µ»
¾
/apis/autoscaling/v1/¤"¡
autoscaling_v1get available resources*getAutoscalingV1APIResourcesB×Ô
200Ì
É
OKÂ
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
øA
-/apis/autoscaling/v1/horizontalpodautoscalersÆA"Ü
autoscaling_v15list or watch objects of kind HorizontalPodAutoscaler*8listAutoscalingV1HorizontalPodAutoscalerForAllNamespacesBÔÑ
200É
Æ
OK¿
d
application/jsonP
NL
J#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerList
q
application/json;stream=watchP
NL
J#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerList
w
#application/vnd.kubernetes.protobufP
NL
J#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerList
„
0application/vnd.kubernetes.protobuf;stream=watchP
NL
J#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerList
d
application/yamlP
NL
J#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerListj
x-kubernetes-actionlist
jb
x-kubernetes-group-version-kind?=group: autoscaling
version: v1
kind: HorizontalPodAutoscaler
jª
§
allowWatchBookmarksquery÷allowWatchBookmarks requests watch events with type "BOOKMARK". Servers that do not implement bookmarks may ignore this flag and bookmarks are sent at the server's discretion. Clients should not assume bookmarks are returned at any specific interval, nor may they assume the server will send any BOOKMARK event during a session. If this is not a watch, this field is ignored.R
 Êbooleanjî	
ë	
continuequeryÇ	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
 Êstringj‡
„
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
 Êstringj‡
„
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
 Êstringjù

ö

limitqueryÔ
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
 Êintegerj»
¸
prettyquery–If 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
 Êstringjú
÷
resourceVersionqueryÌresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
 ÊstringjÙ
Ö
resourceVersionMatchquery¦resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
 Êstringj•
’
sendInitialEventsqueryä
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
 Êbooleanj´
±
shardSelectorqueryˆshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

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
 Êstringj
š
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
 Êintegerj°
­
watchquery‹Watch for changes to the described resources and return them as a stream of add, update, and remove notifications. Specify resourceVersion.R
 Êboolean
Ê¥
D/apis/autoscaling/v1/namespaces/{namespace}/horizontalpodautoscalers€¥"ÿ?
autoscaling_v15list or watch objects of kind HorizontalPodAutoscaler*2listAutoscalingV1NamespacedHorizontalPodAutoscaler2ª
§
allowWatchBookmarksquery÷allowWatchBookmarks requests watch events with type "BOOKMARK". Servers that do not implement bookmarks may ignore this flag and bookmarks are sent at the server's discretion. Clients should not assume bookmarks are returned at any specific interval, nor may they assume the server will send any BOOKMARK event during a session. If this is not a watch, this field is ignored.R
 Êboolean2î	
ë	
continuequeryÇ	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
 Êstring2‡
„
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
 Êstring2‡
„
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
 Êstring2ù

ö

limitqueryÔ
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
 Êinteger2ú
÷
resourceVersionqueryÌresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
 Êstring2Ù
Ö
resourceVersionMatchquery¦resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
 Êstring2•
’
sendInitialEventsqueryä
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
 Êboolean2´
±
shardSelectorqueryˆshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

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
 Êstring2
š
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
 Êinteger2°
­
watchquery‹Watch for changes to the described resources and return them as a stream of add, update, and remove notifications. Specify resourceVersion.R
 ÊbooleanBÔÑ
200É
Æ
OK¿
d
application/jsonP
NL
J#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerList
q
application/json;stream=watchP
NL
J#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerList
w
#application/vnd.kubernetes.protobufP
NL
J#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerList
„
0application/vnd.kubernetes.protobuf;stream=watchP
NL
J#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerList
d
application/yamlP
NL
J#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerListj
x-kubernetes-actionlist
jb
x-kubernetes-group-version-kind?=group: autoscaling
version: v1
kind: HorizontalPodAutoscaler
2×
autoscaling_v1 create a HorizontalPodAutoscaler*4createAutoscalingV1NamespacedHorizontalPodAutoscaler2
š
dryRunqueryøWhen present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
 Êstring2•
’
fieldManagerqueryêfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
 Êstring2Û
Ø
fieldValidationquery­fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
 Êstring:[
YU
S
*/*L
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerBõË
200Ã
À
OK¹
`
application/jsonL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscaler
s
#application/vnd.kubernetes.protobufL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscaler
`
application/yamlL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerÐ
201È
Å
Created¹
`
application/jsonL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscaler
s
#application/vnd.kubernetes.protobufL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscaler
`
application/yamlL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerÑ
202É
Æ
Accepted¹
`
application/jsonL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscaler
s
#application/vnd.kubernetes.protobufL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscaler
`
application/yamlL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerj
x-kubernetes-actionpost
jb
x-kubernetes-group-version-kind?=group: autoscaling
version: v1
kind: HorizontalPodAutoscaler
:€L
autoscaling_v1,delete collection of HorizontalPodAutoscaler*>deleteAutoscalingV1CollectionNamespacedHorizontalPodAutoscaler2î	
ë	
continuequeryÇ	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
 Êstring2
š
dryRunqueryøWhen present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
 Êstring2‡
„
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
 Êstring2ã
à
gracePeriodSecondsquery±The duration in seconds before the object should be deleted. Value must be non-negative integer. The value zero indicates delete immediately. If this value is nil, the default grace period for the specified type will be used. Defaults to a per object value if not specified. zero means delete immediately.R
 Êinteger2¨
¥
0ignoreStoreReadErrorWithClusterBreakingPotentialqueryØif set to true, it will trigger an unsafe deletion of the resource in case the normal deletion flow fails with a corrupt object error. A resource is considered corrupt if it can not be retrieved from the underlying storage successfully because of a) its data can not be transformed e.g. decryption failure, or b) it fails to decode into an object. NOTE: unsafe deletion ignores finalizer constraints, skips precondition checks, and removes the object from the storage. WARNING: This may potentially break the cluster if the workload associated with the resource being unsafe-deleted relies on normal deletion flow. Use only if you REALLY know what you are doing. The default value is false, and the user must opt in to enable itR
 Êboolean2‡
„
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
 Êstring2ù

ö

limitqueryÔ
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
 Êinteger2Ð
Í
orphanDependentsquery Deprecated: please use the PropagationPolicy, this field will be deprecated in 1.7. Should the dependent objects be orphaned. If true/false, the "orphan" finalizer will be added to/removed from the object's finalizers list. Either this field or PropagationPolicy may be set, but not both.R
 Êboolean2‡
„
propagationPolicyquery×Whether and how garbage collection will be performed. Either this field or OrphanDependents may be set, but not both. The default policy is decided by the existing finalizer set in the metadata.finalizers and the resource-specific default policy. Acceptable values are: 'Orphan' - orphan the dependents; 'Background' - allow the garbage collector to delete the dependents in the background; 'Foreground' - a cascading policy that deletes all dependents in the foreground.R
 Êstring2ú
÷
resourceVersionqueryÌresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
 Êstring2Ù
Ö
resourceVersionMatchquery¦resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
 Êstring2•
’
sendInitialEventsqueryä
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
 Êboolean2´
±
shardSelectorqueryˆshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

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
 Êstring2
š
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
 Êinteger:Z
XV
T
*/*M
KI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsB¼¹
200±
®
OK§
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
version: v1
kind: HorizontalPodAutoscaler
ja
_
	namespacepath:object name and auth scope, such as for teams and projects R
 Êstringj»
¸
prettyquery–If 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
 Êstring
êO
K/apis/autoscaling/v1/namespaces/{namespace}/horizontalpodautoscalers/{name}šO"Ä
autoscaling_v1*read the specified HorizontalPodAutoscaler*2readAutoscalingV1NamespacedHorizontalPodAutoscalerBÎË
200Ã
À
OK¹
`
application/jsonL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscaler
s
#application/vnd.kubernetes.protobufL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscaler
`
application/yamlL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerj
x-kubernetes-actionget
jb
x-kubernetes-group-version-kind?=group: autoscaling
version: v1
kind: HorizontalPodAutoscaler
*
autoscaling_v1-replace the specified HorizontalPodAutoscaler*5replaceAutoscalingV1NamespacedHorizontalPodAutoscaler2
š
dryRunqueryøWhen present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
 Êstring2•
’
fieldManagerqueryêfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
 Êstring2Û
Ø
fieldValidationquery­fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
 Êstring:[
YU
S
*/*L
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerB¡Ë
200Ã
À
OK¹
`
application/jsonL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscaler
s
#application/vnd.kubernetes.protobufL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscaler
`
application/yamlL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerÐ
201È
Å
Created¹
`
application/jsonL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscaler
s
#application/vnd.kubernetes.protobufL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscaler
`
application/yamlL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerj
x-kubernetes-actionput
jb
x-kubernetes-group-version-kind?=group: autoscaling
version: v1
kind: HorizontalPodAutoscaler
:ý
autoscaling_v1 delete a HorizontalPodAutoscaler*4deleteAutoscalingV1NamespacedHorizontalPodAutoscaler2
š
dryRunqueryøWhen present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
 Êstring2ã
à
gracePeriodSecondsquery±The duration in seconds before the object should be deleted. Value must be non-negative integer. The value zero indicates delete immediately. If this value is nil, the default grace period for the specified type will be used. Defaults to a per object value if not specified. zero means delete immediately.R
 Êinteger2¨
¥
0ignoreStoreReadErrorWithClusterBreakingPotentialqueryØif set to true, it will trigger an unsafe deletion of the resource in case the normal deletion flow fails with a corrupt object error. A resource is considered corrupt if it can not be retrieved from the underlying storage successfully because of a) its data can not be transformed e.g. decryption failure, or b) it fails to decode into an object. NOTE: unsafe deletion ignores finalizer constraints, skips precondition checks, and removes the object from the storage. WARNING: This may potentially break the cluster if the workload associated with the resource being unsafe-deleted relies on normal deletion flow. Use only if you REALLY know what you are doing. The default value is false, and the user must opt in to enable itR
 Êboolean2Ð
Í
orphanDependentsquery Deprecated: please use the PropagationPolicy, this field will be deprecated in 1.7. Should the dependent objects be orphaned. If true/false, the "orphan" finalizer will be added to/removed from the object's finalizers list. Either this field or PropagationPolicy may be set, but not both.R
 Êboolean2‡
„
propagationPolicyquery×Whether and how garbage collection will be performed. Either this field or OrphanDependents may be set, but not both. The default policy is decided by the existing finalizer set in the metadata.finalizers and the resource-specific default policy. Acceptable values are: 'Orphan' - orphan the dependents; 'Background' - allow the garbage collector to delete the dependents in the background; 'Foreground' - a cascading policy that deletes all dependents in the foreground.R
 Êstring:Z
XV
T
*/*M
KI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsB¢Ë
200Ã
À
OK¹
`
application/jsonL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscaler
s
#application/vnd.kubernetes.protobufL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscaler
`
application/yamlL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerÑ
202É
Æ
Accepted¹
`
application/jsonL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscaler
s
#application/vnd.kubernetes.protobufL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscaler
`
application/yamlL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerj 
x-kubernetes-action	delete
jb
x-kubernetes-group-version-kind?=group: autoscaling
version: v1
kind: HorizontalPodAutoscaler
RÕ
autoscaling_v16partially update the specified HorizontalPodAutoscaler*3patchAutoscalingV1NamespacedHorizontalPodAutoscaler2
š
dryRunqueryøWhen present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
 Êstring2®
«
fieldManagerqueryƒfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint. This field is required for apply requests (application/apply-patch) but optional for non-apply patch types (JsonPatch, MergePatch, StrategicMergePatch).R
 Êstring2Û
Ø
fieldValidationquery­fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
 Êstring2Í
Ê
forcequery¨Force is going to "force" Apply requests. It means user will re-acquire conflicting fields owned by other people. Force flag must be unset for non-apply patch requests.R
 Êboolean:­
ª¥
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
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchB¡Ë
200Ã
À
OK¹
`
application/jsonL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscaler
s
#application/vnd.kubernetes.protobufL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscaler
`
application/yamlL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerÐ
201È
Å
Created¹
`
application/jsonL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscaler
s
#application/vnd.kubernetes.protobufL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscaler
`
application/yamlL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerj
x-kubernetes-actionpatch
jb
x-kubernetes-group-version-kind?=group: autoscaling
version: v1
kind: HorizontalPodAutoscaler
jE
C
namepath#name of the HorizontalPodAutoscaler R
 Êstringja
_
	namespacepath:object name and auth scope, such as for teams and projects R
 Êstringj»
¸
prettyquery–If 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
 Êstring
¡6
R/apis/autoscaling/v1/namespaces/{namespace}/horizontalpodautoscalers/{name}/statusÊ5"Ô
autoscaling_v14read status of the specified HorizontalPodAutoscaler*8readAutoscalingV1NamespacedHorizontalPodAutoscalerStatusBÎË
200Ã
À
OK¹
`
application/jsonL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscaler
s
#application/vnd.kubernetes.protobufL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscaler
`
application/yamlL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerj
x-kubernetes-actionget
jb
x-kubernetes-group-version-kind?=group: autoscaling
version: v1
kind: HorizontalPodAutoscaler
* 
autoscaling_v17replace status of the specified HorizontalPodAutoscaler*;replaceAutoscalingV1NamespacedHorizontalPodAutoscalerStatus2
š
dryRunqueryøWhen present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
 Êstring2•
’
fieldManagerqueryêfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
 Êstring2Û
Ø
fieldValidationquery­fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
 Êstring:[
YU
S
*/*L
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerB¡Ë
200Ã
À
OK¹
`
application/jsonL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscaler
s
#application/vnd.kubernetes.protobufL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscaler
`
application/yamlL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerÐ
201È
Å
Created¹
`
application/jsonL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscaler
s
#application/vnd.kubernetes.protobufL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscaler
`
application/yamlL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerj
x-kubernetes-actionput
jb
x-kubernetes-group-version-kind?=group: autoscaling
version: v1
kind: HorizontalPodAutoscaler
Rå
autoscaling_v1@partially update status of the specified HorizontalPodAutoscaler*9patchAutoscalingV1NamespacedHorizontalPodAutoscalerStatus2
š
dryRunqueryøWhen present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
 Êstring2®
«
fieldManagerqueryƒfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint. This field is required for apply requests (application/apply-patch) but optional for non-apply patch types (JsonPatch, MergePatch, StrategicMergePatch).R
 Êstring2Û
Ø
fieldValidationquery­fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
 Êstring2Í
Ê
forcequery¨Force is going to "force" Apply requests. It means user will re-acquire conflicting fields owned by other people. Force flag must be unset for non-apply patch requests.R
 Êboolean:­
ª¥
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
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchB¡Ë
200Ã
À
OK¹
`
application/jsonL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscaler
s
#application/vnd.kubernetes.protobufL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscaler
`
application/yamlL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerÐ
201È
Å
Created¹
`
application/jsonL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscaler
s
#application/vnd.kubernetes.protobufL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscaler
`
application/yamlL
JH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerj
x-kubernetes-actionpatch
jb
x-kubernetes-group-version-kind?=group: autoscaling
version: v1
kind: HorizontalPodAutoscaler
jE
C
namepath#name of the HorizontalPodAutoscaler R
 Êstringja
_
	namespacepath:object name and auth scope, such as for teams and projects R
 Êstringj»
¸
prettyquery–If 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
 Êstring
¸B
3/apis/autoscaling/v1/watch/horizontalpodautoscalers€B"–
autoscaling_v1ƒwatch individual changes to a list of HorizontalPodAutoscaler. deprecated: use the 'watch' parameter with a list operation instead.*=watchAutoscalingV1HorizontalPodAutoscalerListForAllNamespacesBµ²
200ª
§
OK 
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
version: v1
kind: HorizontalPodAutoscaler
jª
§
allowWatchBookmarksquery÷allowWatchBookmarks requests watch events with type "BOOKMARK". Servers that do not implement bookmarks may ignore this flag and bookmarks are sent at the server's discretion. Clients should not assume bookmarks are returned at any specific interval, nor may they assume the server will send any BOOKMARK event during a session. If this is not a watch, this field is ignored.R
 Êbooleanjî	
ë	
continuequeryÇ	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
 Êstringj‡
„
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
 Êstringj‡
„
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
 Êstringjù

ö

limitqueryÔ
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
 Êintegerj»
¸
prettyquery–If 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
 Êstringjú
÷
resourceVersionqueryÌresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
 ÊstringjÙ
Ö
resourceVersionMatchquery¦resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
 Êstringj•
’
sendInitialEventsqueryä
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
 Êbooleanj´
±
shardSelectorqueryˆshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

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
 Êstringj
š
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
 Êintegerj°
­
watchquery‹Watch for changes to the described resources and return them as a stream of add, update, and remove notifications. Specify resourceVersion.R
 Êboolean
¬C
J/apis/autoscaling/v1/watch/namespaces/{namespace}/horizontalpodautoscalersÝB"
autoscaling_v1ƒwatch individual changes to a list of HorizontalPodAutoscaler. deprecated: use the 'watch' parameter with a list operation instead.*7watchAutoscalingV1NamespacedHorizontalPodAutoscalerListBµ²
200ª
§
OK 
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
version: v1
kind: HorizontalPodAutoscaler
jª
§
allowWatchBookmarksquery÷allowWatchBookmarks requests watch events with type "BOOKMARK". Servers that do not implement bookmarks may ignore this flag and bookmarks are sent at the server's discretion. Clients should not assume bookmarks are returned at any specific interval, nor may they assume the server will send any BOOKMARK event during a session. If this is not a watch, this field is ignored.R
 Êbooleanjî	
ë	
continuequeryÇ	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
 Êstringj‡
„
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
 Êstringj‡
„
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
 Êstringjù

ö

limitqueryÔ
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
 Êintegerja
_
	namespacepath:object name and auth scope, such as for teams and projects R
 Êstringj»
¸
prettyquery–If 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
 Êstringjú
÷
resourceVersionqueryÌresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
 ÊstringjÙ
Ö
resourceVersionMatchquery¦resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
 Êstringj•
’
sendInitialEventsqueryä
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
 Êbooleanj´
±
shardSelectorqueryˆshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

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
 Êstringj
š
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
 Êintegerj°
­
watchquery‹Watch for changes to the described resources and return them as a stream of add, update, and remove notifications. Specify resourceVersion.R
 Êboolean
­D
Q/apis/autoscaling/v1/watch/namespaces/{namespace}/horizontalpodautoscalers/{name}×C"Ã
autoscaling_v1¾watch changes to an object of kind HorizontalPodAutoscaler. deprecated: use the 'watch' parameter with a list operation instead, filtered to a single item with the 'fieldSelector' parameter.*3watchAutoscalingV1NamespacedHorizontalPodAutoscalerBµ²
200ª
§
OK 
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
version: v1
kind: HorizontalPodAutoscaler
jª
§
allowWatchBookmarksquery÷allowWatchBookmarks requests watch events with type "BOOKMARK". Servers that do not implement bookmarks may ignore this flag and bookmarks are sent at the server's discretion. Clients should not assume bookmarks are returned at any specific interval, nor may they assume the server will send any BOOKMARK event during a session. If this is not a watch, this field is ignored.R
 Êbooleanjî	
ë	
continuequeryÇ	The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server, the server will respond with a 410 ResourceExpired error together with a continue token. If the client needs a consistent list, it must restart their list without the continue field. Otherwise, the client may send another list request with the token received with the 410 error, the server will respond with a list starting from the next key, but from the latest snapshot, which is inconsistent from the previous list results - objects that are created, modified, or deleted after the first list request will be included in the response, as long as their keys are after the "next key".

This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.R
 Êstringj‡
„
fieldSelectorquery\A selector to restrict the list of returned objects by their fields. Defaults to everything.R
 Êstringj‡
„
labelSelectorquery\A selector to restrict the list of returned objects by their labels. Defaults to everything.R
 Êstringjù

ö

limitqueryÔ
limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.

The server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.R
 ÊintegerjE
C
namepath#name of the HorizontalPodAutoscaler R
 Êstringja
_
	namespacepath:object name and auth scope, such as for teams and projects R
 Êstringj»
¸
prettyquery–If 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
 Êstringjú
÷
resourceVersionqueryÌresourceVersion sets a constraint on what resource versions a request may be served from. See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
 ÊstringjÙ
Ö
resourceVersionMatchquery¦resourceVersionMatch determines how resourceVersion is applied to list calls. It is highly recommended that resourceVersionMatch be set for list calls where resourceVersion is set See https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions for details.

Defaults to unsetR
 Êstringj•
’
sendInitialEventsqueryä
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
 Êbooleanj´
±
shardSelectorqueryˆshardSelector restricts the list of returned objects using a CEL-based shard selector expression. The format uses the shardRange() function combined with || (logical OR) to specify one or more hash ranges:

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
 Êstringj
š
timeoutSecondsquerypTimeout for the list/watch call. This limits the duration of the call, regardless of any activity or inactivity.R
 Êintegerj°
­
watchquery‹Watch for changes to the described resources and return them as a stream of add, update, and remove notifications. Specify resourceVersion.R
 Êboolean*ÿ9
ü9
Ë
5io.k8s.api.autoscaling.v1.CrossVersionObjectReference‘
ŽºkindºnameÊobjectúO


apiVersion
	Êstring

kind
ÊstringŠ 

name
ÊstringŠ ¢"
x-kubernetes-map-type	atomic

Š
1io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerÔ
ÑºspecÊobjectúÒ


apiVersion
	Êstring

kind
	Êstring
\
metadataP
NÒHF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMetaŠ 
^
specV
TÒNL
J#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerSpecŠ 
b
statusX
VÒPN
L#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerStatusŠ ¢h
x-kubernetes-group-version-kindEC- group: autoscaling
  kind: HorizontalPodAutoscaler
  version: v1

¹
5io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerListÿ
üºitemsÊobjectúø


apiVersion
	Êstring
j
itemsa
_ÊarrayòT
R
PÒJH
F#/components/schemas/io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerŠ 

kind
	Êstring
Z
metadataN
LÒFD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaŠ ¢l
x-kubernetes-group-version-kindIG- group: autoscaling
  kind: HorizontalPodAutoscalerList
  version: v1

á
5io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerSpec§
¤ºscaleTargetRefºmaxReplicasÊobjectúø
/
maxReplicas 
ÊintegerŠ		        šint32
#
minReplicas
Êintegeršint32
h
scaleTargetRefV
TÒNL
J#/components/schemas/io.k8s.api.autoscaling.v1.CrossVersionObjectReferenceŠ 
6
targetCPUUtilizationPercentage
Êintegeršint32
”
7io.k8s.api.autoscaling.v1.HorizontalPodAutoscalerStatusØ
ÕºcurrentReplicasºdesiredReplicasÊobjectú¤
7
currentCPUUtilizationPercentage
Êintegeršint32
3
currentReplicas 
ÊintegerŠ		        šint32
3
desiredReplicas 
ÊintegerŠ		        šint32
S
lastScaleTimeB@
>#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Time
*
observedGeneration
Êintegeršint64
“
0io.k8s.apimachinery.pkg.apis.meta.v1.APIResourceÞ
ÛºnameºsingularNameº
namespacedºkindºverbsÊobjectúœ
S

categoriesE
CÊarrayò

ÊstringŠ ¢#
x-kubernetes-list-type	atomic


group
	Êstring

kind
ÊstringŠ 

name
ÊstringŠ 


namespaced
ÊbooleanŠ 
S

shortNamesE
CÊarrayò

ÊstringŠ ¢#
x-kubernetes-list-type	atomic

 
singularName
ÊstringŠ 
!
storageVersionHash
	Êstring
(
verbs
Êarrayò

ÊstringŠ 

version
	Êstring
¨
4io.k8s.apimachinery.pkg.apis.meta.v1.APIResourceListï
ìºgroupVersionº	resourcesÊobjectúê


apiVersion
	Êstring
 
groupVersion
ÊstringŠ 

kind
	Êstring
•
	resources‡
„ÊarrayòS
Q
OÒIG
E#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.APIResourceŠ ¢#
x-kubernetes-list-type	atomic
¢W
x-kubernetes-group-version-kind42- group: ""
  kind: APIResourceList
  version: v1

ê
2io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptions³
°Êobjectú‘


apiVersion
	Êstring
O
dryRunE
CÊarrayò

ÊstringŠ ¢#
x-kubernetes-list-type	atomic

*
gracePeriodSeconds
Êintegeršint64
@
0ignoreStoreReadErrorWithClusterBreakingPotential

Êboolean

kind
	Êstring
 
orphanDependents

Êboolean
\
preconditionsKI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Preconditions
 
propagationPolicy
	Êstring¢Ž
x-kubernetes-group-version-kindki- group: ""
  kind: DeleteOptions
  version: v1
- group: autoscaling
  kind: DeleteOptions
  version: v1

<
-io.k8s.apimachinery.pkg.apis.meta.v1.FieldsV1
	Êobject
–
-io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaä
áÊobjectúÔ

continue
	Êstring
*
remainingItemCount
Êintegeršint64

resourceVersion
	Êstring

selfLink
	Êstring
T
	shardInfoGE
C#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ShardInfo
ð
7io.k8s.apimachinery.pkg.apis.meta.v1.ManagedFieldsEntry´
±Êobjectú¤


apiVersion
	Êstring


fieldsType
	Êstring
R
fieldsV1FD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.FieldsV1

manager
	Êstring

	operation
	Êstring

subresource
	Êstring
J
timeB@
>#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Time
ý
/io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMetaÉ
ÆÊobjectú¹
/
annotations 
Êobject‚

ÊstringŠ 
W
creationTimestampB@
>#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Time
2
deletionGracePeriodSeconds
Êintegeršint64
W
deletionTimestampB@
>#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Time
z

finalizersl
jÊarrayò

ÊstringŠ ¢ 
x-kubernetes-list-typeset
¢'
x-kubernetes-patch-strategymerge


generateName
	Êstring
"

generation
Êintegeršint64
*
labels 
Êobject‚

ÊstringŠ 
 
managedFieldsŽ
‹ÊarrayòZ
X
VÒPN
L#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ManagedFieldsEntryŠ ¢#
x-kubernetes-list-type	atomic


name
	Êstring

	namespace
	Êstring
—
ownerReferencesƒ
€ÊarrayòV
T
RÒLJ
H#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.OwnerReferenceŠ ¢&
x-kubernetes-list-map-keys- uid
¢ 
x-kubernetes-list-typemap
¢&
x-kubernetes-patch-merge-keyuid
¢'
x-kubernetes-patch-strategymerge


resourceVersion
	Êstring

selfLink
	Êstring

uid
	Êstring
»
3io.k8s.apimachinery.pkg.apis.meta.v1.OwnerReferenceƒ
€º
apiVersionºkindºnameºuidÊobjectú­


apiVersion
ÊstringŠ 
"
blockOwnerDeletion

Êboolean


controller

Êboolean

kind
ÊstringŠ 

name
ÊstringŠ 

uid
ÊstringŠ ¢"
x-kubernetes-map-type	atomic

9
*io.k8s.apimachinery.pkg.apis.meta.v1.Patch
	Êobject
x
2io.k8s.apimachinery.pkg.apis.meta.v1.PreconditionsB
@Êobjectú4

resourceVersion
	Êstring

uid
	Êstring
i
.io.k8s.apimachinery.pkg.apis.meta.v1.ShardInfo7
5ºselectorÊobjectú

selector
ÊstringŠ 
Ù
+io.k8s.apimachinery.pkg.apis.meta.v1.Status©
¦ÊobjectúÈ


apiVersion
	Êstring

code
Êintegeršint32
V
detailsKI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.StatusDetails

kind
	Êstring

message
	Êstring
Z
metadataN
LÒFD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaŠ 

reason
	Êstring

status
	Êstring¢N
x-kubernetes-group-version-kind+)- group: ""
  kind: Status
  version: v1

‡
0io.k8s.apimachinery.pkg.apis.meta.v1.StatusCauseS
QÊobjectúE

field
	Êstring

message
	Êstring

reason
	Êstring
Û
2io.k8s.apimachinery.pkg.apis.meta.v1.StatusDetails¤
¡Êobjectú”
’
causes‡
„ÊarrayòS
Q
OÒIG
E#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.StatusCauseŠ ¢#
x-kubernetes-list-type	atomic


group
	Êstring

kind
	Êstring

name
	Êstring
)
retryAfterSeconds
Êintegeršint32

uid
	Êstring
D
)io.k8s.apimachinery.pkg.apis.meta.v1.Time
Êstringš	date-time
Ê
/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent–
“ºtypeºobjectÊobjectúk
O
objectEC
A#/components/schemas/io.k8s.apimachinery.pkg.runtime.RawExtension

type
ÊstringŠ ¢ˆ
x-kubernetes-group-version-kindec- group: ""
  kind: WatchEvent
  version: v1
- group: autoscaling
  kind: WatchEvent
  version: v1

;
,io.k8s.apimachinery.pkg.runtime.RawExtension
	Êobject