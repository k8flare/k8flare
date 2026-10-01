
3.0.0

Kubernetes2v1.36.4+k8flare"ì 
¨
/apis/batch/v1/ò"ï
batch_v1get available resources*getBatchV1APIResourcesB◊‘
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
±@
/apis/batch/v1/cronjobsï@"´
batch_v1%list or watch objects of kind CronJob*"listBatchV1CronJobForAllNamespacesBÂ‚
200⁄
◊
OK–
N
application/json:
86
4#/components/schemas/io.k8s.api.batch.v1.CronJobList
[
application/json;stream=watch:
86
4#/components/schemas/io.k8s.api.batch.v1.CronJobList
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.batch.v1.CronJobList
n
0application/vnd.kubernetes.protobuf;stream=watch:
86
4#/components/schemas/io.k8s.api.batch.v1.CronJobList
N
application/yaml:
86
4#/components/schemas/io.k8s.api.batch.v1.CronJobListj
x-kubernetes-actionlist
jL
x-kubernetes-group-version-kind)'group: batch
version: v1
kind: CronJob
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
ç@
/apis/batch/v1/jobsı?"ã
batch_v1!list or watch objects of kind Job*listBatchV1JobForAllNamespacesB—Œ
200∆
√
OKº
J
application/json6
42
0#/components/schemas/io.k8s.api.batch.v1.JobList
W
application/json;stream=watch6
42
0#/components/schemas/io.k8s.api.batch.v1.JobList
]
#application/vnd.kubernetes.protobuf6
42
0#/components/schemas/io.k8s.api.batch.v1.JobList
j
0application/vnd.kubernetes.protobuf;stream=watch6
42
0#/components/schemas/io.k8s.api.batch.v1.JobList
J
application/yaml6
42
0#/components/schemas/io.k8s.api.batch.v1.JobListj
x-kubernetes-actionlist
jH
x-kubernetes-group-version-kind%#group: batch
version: v1
kind: Job
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
£°
./apis/batch/v1/namespaces/{namespace}/cronjobsÔ†"Œ>
batch_v1%list or watch objects of kind CronJob*listBatchV1NamespacedCronJob2™
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
† booleanBÂ‚
200⁄
◊
OK–
N
application/json:
86
4#/components/schemas/io.k8s.api.batch.v1.CronJobList
[
application/json;stream=watch:
86
4#/components/schemas/io.k8s.api.batch.v1.CronJobList
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.batch.v1.CronJobList
n
0application/vnd.kubernetes.protobuf;stream=watch:
86
4#/components/schemas/io.k8s.api.batch.v1.CronJobList
N
application/yaml:
86
4#/components/schemas/io.k8s.api.batch.v1.CronJobListj
x-kubernetes-actionlist
jL
x-kubernetes-group-version-kind)'group: batch
version: v1
kind: CronJob
2π
batch_v1create a CronJob*createBatchV1NamespacedCronJob2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:E
C?
=
*/*6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJobBØâ
200Å
˛
OK˜
J
application/json6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJob
]
#application/vnd.kubernetes.protobuf6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJob
J
application/yaml6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJobé
201Ü
É
Created˜
J
application/json6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJob
]
#application/vnd.kubernetes.protobuf6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJob
J
application/yaml6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJobè
202á
Ñ
Accepted˜
J
application/json6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJob
]
#application/vnd.kubernetes.protobuf6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJob
J
application/yaml6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJobj
x-kubernetes-actionpost
jL
x-kubernetes-group-version-kind)'group: batch
version: v1
kind: CronJob
:æK
batch_v1delete collection of CronJob*(deleteBatchV1CollectionNamespacedCronJob2Ó	
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
jL
x-kubernetes-group-version-kind)'group: batch
version: v1
kind: CronJob
ja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
ÿI
5/apis/batch/v1/namespaces/{namespace}/cronjobs/{name}ûI"¿
batch_v1read the specified CronJob*readBatchV1NamespacedCronJobBåâ
200Å
˛
OK˜
J
application/json6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJob
]
#application/vnd.kubernetes.protobuf6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJob
J
application/yaml6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJobj
x-kubernetes-actionget
jL
x-kubernetes-group-version-kind)'group: batch
version: v1
kind: CronJob
*¥
batch_v1replace the specified CronJob*replaceBatchV1NamespacedCronJob2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:E
C?
=
*/*6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJobBùâ
200Å
˛
OK˜
J
application/json6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJob
]
#application/vnd.kubernetes.protobuf6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJob
J
application/yaml6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJobé
201Ü
É
Created˜
J
application/json6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJob
]
#application/vnd.kubernetes.protobuf6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJob
J
application/yaml6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJobj
x-kubernetes-actionput
jL
x-kubernetes-group-version-kind)'group: batch
version: v1
kind: CronJob
:∑
batch_v1delete a CronJob*deleteBatchV1NamespacedCronJob2ù
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
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsBûâ
200Å
˛
OK˜
J
application/json6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJob
]
#application/vnd.kubernetes.protobuf6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJob
J
application/yaml6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJobè
202á
Ñ
Accepted˜
J
application/json6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJob
]
#application/vnd.kubernetes.protobuf6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJob
J
application/yaml6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJobj 
x-kubernetes-action	delete
jL
x-kubernetes-group-version-kind)'group: batch
version: v1
kind: CronJob
Rè
batch_v1&partially update the specified CronJob*patchBatchV1NamespacedCronJob2ù
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
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchBùâ
200Å
˛
OK˜
J
application/json6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJob
]
#application/vnd.kubernetes.protobuf6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJob
J
application/yaml6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJobé
201Ü
É
Created˜
J
application/json6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJob
]
#application/vnd.kubernetes.protobuf6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJob
J
application/yaml6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJobj
x-kubernetes-actionpatch
jL
x-kubernetes-group-version-kind)'group: batch
version: v1
kind: CronJob
j5
3
namepathname of the CronJob R
† stringja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
’1
</apis/batch/v1/namespaces/{namespace}/cronjobs/{name}/statusî1"–
batch_v1$read status of the specified CronJob*"readBatchV1NamespacedCronJobStatusBåâ
200Å
˛
OK˜
J
application/json6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJob
]
#application/vnd.kubernetes.protobuf6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJob
J
application/yaml6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJobj
x-kubernetes-actionget
jL
x-kubernetes-group-version-kind)'group: batch
version: v1
kind: CronJob
*ƒ
batch_v1'replace status of the specified CronJob*%replaceBatchV1NamespacedCronJobStatus2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:E
C?
=
*/*6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJobBùâ
200Å
˛
OK˜
J
application/json6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJob
]
#application/vnd.kubernetes.protobuf6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJob
J
application/yaml6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJobé
201Ü
É
Created˜
J
application/json6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJob
]
#application/vnd.kubernetes.protobuf6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJob
J
application/yaml6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJobj
x-kubernetes-actionput
jL
x-kubernetes-group-version-kind)'group: batch
version: v1
kind: CronJob
Rü
batch_v10partially update status of the specified CronJob*#patchBatchV1NamespacedCronJobStatus2ù
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
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchBùâ
200Å
˛
OK˜
J
application/json6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJob
]
#application/vnd.kubernetes.protobuf6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJob
J
application/yaml6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJobé
201Ü
É
Created˜
J
application/json6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJob
]
#application/vnd.kubernetes.protobuf6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJob
J
application/yaml6
42
0#/components/schemas/io.k8s.api.batch.v1.CronJobj
x-kubernetes-actionpatch
jL
x-kubernetes-group-version-kind)'group: batch
version: v1
kind: CronJob
j5
3
namepathname of the CronJob R
† stringja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
ø†
*/apis/batch/v1/namespaces/{namespace}/jobsè†"Æ>
batch_v1!list or watch objects of kind Job*listBatchV1NamespacedJob2™
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
† booleanB—Œ
200∆
√
OKº
J
application/json6
42
0#/components/schemas/io.k8s.api.batch.v1.JobList
W
application/json;stream=watch6
42
0#/components/schemas/io.k8s.api.batch.v1.JobList
]
#application/vnd.kubernetes.protobuf6
42
0#/components/schemas/io.k8s.api.batch.v1.JobList
j
0application/vnd.kubernetes.protobuf;stream=watch6
42
0#/components/schemas/io.k8s.api.batch.v1.JobList
J
application/yaml6
42
0#/components/schemas/io.k8s.api.batch.v1.JobListj
x-kubernetes-actionlist
jH
x-kubernetes-group-version-kind%#group: batch
version: v1
kind: Job
2Ö
batch_v1create a Job*createBatchV1NamespacedJob2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:A
?;
9
*/*2
0.
,#/components/schemas/io.k8s.api.batch.v1.JobBã˝
200ı
Ú
OKÎ
F
application/json2
0.
,#/components/schemas/io.k8s.api.batch.v1.Job
Y
#application/vnd.kubernetes.protobuf2
0.
,#/components/schemas/io.k8s.api.batch.v1.Job
F
application/yaml2
0.
,#/components/schemas/io.k8s.api.batch.v1.JobÇ
201˙
˜
CreatedÎ
F
application/json2
0.
,#/components/schemas/io.k8s.api.batch.v1.Job
Y
#application/vnd.kubernetes.protobuf2
0.
,#/components/schemas/io.k8s.api.batch.v1.Job
F
application/yaml2
0.
,#/components/schemas/io.k8s.api.batch.v1.JobÉ
202˚
¯
AcceptedÎ
F
application/json2
0.
,#/components/schemas/io.k8s.api.batch.v1.Job
Y
#application/vnd.kubernetes.protobuf2
0.
,#/components/schemas/io.k8s.api.batch.v1.Job
F
application/yaml2
0.
,#/components/schemas/io.k8s.api.batch.v1.Jobj
x-kubernetes-actionpost
jH
x-kubernetes-group-version-kind%#group: batch
version: v1
kind: Job
:≤K
batch_v1delete collection of Job*$deleteBatchV1CollectionNamespacedJob2Ó	
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
jH
x-kubernetes-group-version-kind%#group: batch
version: v1
kind: Job
ja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
»H
1/apis/batch/v1/namespaces/{namespace}/jobs/{name}íH"®
batch_v1read the specified Job*readBatchV1NamespacedJobBÄ˝
200ı
Ú
OKÎ
F
application/json2
0.
,#/components/schemas/io.k8s.api.batch.v1.Job
Y
#application/vnd.kubernetes.protobuf2
0.
,#/components/schemas/io.k8s.api.batch.v1.Job
F
application/yaml2
0.
,#/components/schemas/io.k8s.api.batch.v1.Jobj
x-kubernetes-actionget
jH
x-kubernetes-group-version-kind%#group: batch
version: v1
kind: Job
*å
batch_v1replace the specified Job*replaceBatchV1NamespacedJob2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:A
?;
9
*/*2
0.
,#/components/schemas/io.k8s.api.batch.v1.JobBÖ˝
200ı
Ú
OKÎ
F
application/json2
0.
,#/components/schemas/io.k8s.api.batch.v1.Job
Y
#application/vnd.kubernetes.protobuf2
0.
,#/components/schemas/io.k8s.api.batch.v1.Job
F
application/yaml2
0.
,#/components/schemas/io.k8s.api.batch.v1.JobÇ
201˙
˜
CreatedÎ
F
application/json2
0.
,#/components/schemas/io.k8s.api.batch.v1.Job
Y
#application/vnd.kubernetes.protobuf2
0.
,#/components/schemas/io.k8s.api.batch.v1.Job
F
application/yaml2
0.
,#/components/schemas/io.k8s.api.batch.v1.Jobj
x-kubernetes-actionput
jH
x-kubernetes-group-version-kind%#group: batch
version: v1
kind: Job
:ì
batch_v1delete a Job*deleteBatchV1NamespacedJob2ù
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
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsBÜ˝
200ı
Ú
OKÎ
F
application/json2
0.
,#/components/schemas/io.k8s.api.batch.v1.Job
Y
#application/vnd.kubernetes.protobuf2
0.
,#/components/schemas/io.k8s.api.batch.v1.Job
F
application/yaml2
0.
,#/components/schemas/io.k8s.api.batch.v1.JobÉ
202˚
¯
AcceptedÎ
F
application/json2
0.
,#/components/schemas/io.k8s.api.batch.v1.Job
Y
#application/vnd.kubernetes.protobuf2
0.
,#/components/schemas/io.k8s.api.batch.v1.Job
F
application/yaml2
0.
,#/components/schemas/io.k8s.api.batch.v1.Jobj 
x-kubernetes-action	delete
jH
x-kubernetes-group-version-kind%#group: batch
version: v1
kind: Job
RÎ
batch_v1"partially update the specified Job*patchBatchV1NamespacedJob2ù
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
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchBÖ˝
200ı
Ú
OKÎ
F
application/json2
0.
,#/components/schemas/io.k8s.api.batch.v1.Job
Y
#application/vnd.kubernetes.protobuf2
0.
,#/components/schemas/io.k8s.api.batch.v1.Job
F
application/yaml2
0.
,#/components/schemas/io.k8s.api.batch.v1.JobÇ
201˙
˜
CreatedÎ
F
application/json2
0.
,#/components/schemas/io.k8s.api.batch.v1.Job
Y
#application/vnd.kubernetes.protobuf2
0.
,#/components/schemas/io.k8s.api.batch.v1.Job
F
application/yaml2
0.
,#/components/schemas/io.k8s.api.batch.v1.Jobj
x-kubernetes-actionpatch
jH
x-kubernetes-group-version-kind%#group: batch
version: v1
kind: Job
j1
/
namepathname of the Job R
† stringja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
È0
8/apis/batch/v1/namespaces/{namespace}/jobs/{name}/status¨0"∏
batch_v1 read status of the specified Job*readBatchV1NamespacedJobStatusBÄ˝
200ı
Ú
OKÎ
F
application/json2
0.
,#/components/schemas/io.k8s.api.batch.v1.Job
Y
#application/vnd.kubernetes.protobuf2
0.
,#/components/schemas/io.k8s.api.batch.v1.Job
F
application/yaml2
0.
,#/components/schemas/io.k8s.api.batch.v1.Jobj
x-kubernetes-actionget
jH
x-kubernetes-group-version-kind%#group: batch
version: v1
kind: Job
*ú
batch_v1#replace status of the specified Job*!replaceBatchV1NamespacedJobStatus2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:A
?;
9
*/*2
0.
,#/components/schemas/io.k8s.api.batch.v1.JobBÖ˝
200ı
Ú
OKÎ
F
application/json2
0.
,#/components/schemas/io.k8s.api.batch.v1.Job
Y
#application/vnd.kubernetes.protobuf2
0.
,#/components/schemas/io.k8s.api.batch.v1.Job
F
application/yaml2
0.
,#/components/schemas/io.k8s.api.batch.v1.JobÇ
201˙
˜
CreatedÎ
F
application/json2
0.
,#/components/schemas/io.k8s.api.batch.v1.Job
Y
#application/vnd.kubernetes.protobuf2
0.
,#/components/schemas/io.k8s.api.batch.v1.Job
F
application/yaml2
0.
,#/components/schemas/io.k8s.api.batch.v1.Jobj
x-kubernetes-actionput
jH
x-kubernetes-group-version-kind%#group: batch
version: v1
kind: Job
R˚
batch_v1,partially update status of the specified Job*patchBatchV1NamespacedJobStatus2ù
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
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchBÖ˝
200ı
Ú
OKÎ
F
application/json2
0.
,#/components/schemas/io.k8s.api.batch.v1.Job
Y
#application/vnd.kubernetes.protobuf2
0.
,#/components/schemas/io.k8s.api.batch.v1.Job
F
application/yaml2
0.
,#/components/schemas/io.k8s.api.batch.v1.JobÇ
201˙
˜
CreatedÎ
F
application/json2
0.
,#/components/schemas/io.k8s.api.batch.v1.Job
Y
#application/vnd.kubernetes.protobuf2
0.
,#/components/schemas/io.k8s.api.batch.v1.Job
F
application/yaml2
0.
,#/components/schemas/io.k8s.api.batch.v1.Jobj
x-kubernetes-actionpatch
jH
x-kubernetes-group-version-kind%#group: batch
version: v1
kind: Job
j1
/
namepathname of the Job R
† stringja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
ﬂA
/apis/batch/v1/watch/cronjobsΩA"”
batch_v1swatch individual changes to a list of CronJob. deprecated: use the 'watch' parameter with a list operation instead.*'watchBatchV1CronJobListForAllNamespacesBµ≤
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
jL
x-kubernetes-group-version-kind)'group: batch
version: v1
kind: CronJob
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
œA
/apis/batch/v1/watch/jobs±A"«
batch_v1owatch individual changes to a list of Job. deprecated: use the 'watch' parameter with a list operation instead.*#watchBatchV1JobListForAllNamespacesBµ≤
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
jH
x-kubernetes-group-version-kind%#group: batch
version: v1
kind: Job
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
”B
4/apis/batch/v1/watch/namespaces/{namespace}/cronjobsöB"Õ
batch_v1swatch individual changes to a list of CronJob. deprecated: use the 'watch' parameter with a list operation instead.*!watchBatchV1NamespacedCronJobListBµ≤
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
jL
x-kubernetes-group-version-kind)'group: batch
version: v1
kind: CronJob
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
≈C
;/apis/batch/v1/watch/namespaces/{namespace}/cronjobs/{name}ÖC"Å
batch_v1Æwatch changes to an object of kind CronJob. deprecated: use the 'watch' parameter with a list operation instead, filtered to a single item with the 'fieldSelector' parameter.*watchBatchV1NamespacedCronJobBµ≤
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
jL
x-kubernetes-group-version-kind)'group: batch
version: v1
kind: CronJob
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
† integerj5
3
namepathname of the CronJob R
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
† boolean
√B
0/apis/batch/v1/watch/namespaces/{namespace}/jobséB"¡
batch_v1owatch individual changes to a list of Job. deprecated: use the 'watch' parameter with a list operation instead.*watchBatchV1NamespacedJobListBµ≤
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
jH
x-kubernetes-group-version-kind%#group: batch
version: v1
kind: Job
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
±C
7/apis/batch/v1/watch/namespaces/{namespace}/jobs/{name}ıB"ı
batch_v1™watch changes to an object of kind Job. deprecated: use the 'watch' parameter with a list operation instead, filtered to a single item with the 'fieldSelector' parameter.*watchBatchV1NamespacedJobBµ≤
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
jH
x-kubernetes-group-version-kind%#group: batch
version: v1
kind: Job
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
† integerj1
/
namepathname of the Job R
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
† boolean*Áì
„ì
≤
io.k8s.api.batch.v1.CronJobí
è∫spec object˙¶
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
H
spec@
>“86
4#/components/schemas/io.k8s.api.batch.v1.CronJobSpecä 
L
statusB
@“:8
6#/components/schemas/io.k8s.api.batch.v1.CronJobStatusä ¢R
x-kubernetes-group-version-kind/-- group: batch
  kind: CronJob
  version: v1

˜
io.k8s.api.batch.v1.CronJobList”
–∫items object˙‚


apiVersion
	 string
T
itemsK
I arrayÚ>
<
:“42
0#/components/schemas/io.k8s.api.batch.v1.CronJobä 

kind
	 string
Z
metadataN
L“FD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaä ¢V
x-kubernetes-group-version-kind31- group: batch
  kind: CronJobList
  version: v1

Õ
io.k8s.api.batch.v1.CronJobSpec©
¶∫schedule∫jobTemplate object˙Ä
D
concurrencyPolicy/
-¬Allow
¬	Forbid
¬
Replace
 string
.
failedJobsHistoryLimit
 integeröint32
S
jobTemplateD
B“<:
8#/components/schemas/io.k8s.api.batch.v1.JobTemplateSpecä 

schedule
 stringä 
/
startingDeadlineSeconds
 integeröint64
2
successfulJobsHistoryLimit
 integeröint32

suspend

 boolean

timeZone
	 string
Ì
!io.k8s.api.batch.v1.CronJobStatus«
ƒ object˙∑
Ç
activex
v arrayÚE
C
A“;9
7#/components/schemas/io.k8s.api.core.v1.ObjectReferenceä ¢#
x-kubernetes-list-type	atomic

V
lastScheduleTimeB@
>#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Time
X
lastSuccessfulTimeB@
>#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Time
õ
io.k8s.api.batch.v1.Jobˇ
¸ object˙û
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
D
spec<
:“42
0#/components/schemas/io.k8s.api.batch.v1.JobSpecä 
H
status>
<“64
2#/components/schemas/io.k8s.api.batch.v1.JobStatusä ¢N
x-kubernetes-group-version-kind+)- group: batch
  kind: Job
  version: v1

Ÿ
 io.k8s.api.batch.v1.JobCondition¥
±∫type∫status object˙î
S
lastProbeTimeB@
>#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Time
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
Î
io.k8s.api.batch.v1.JobListÀ
»∫items object˙ﬁ


apiVersion
	 string
P
itemsG
E arrayÚ:
8
6“0.
,#/components/schemas/io.k8s.api.batch.v1.Jobä 

kind
	 string
Z
metadataN
L“FD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaä ¢R
x-kubernetes-group-version-kind/-- group: batch
  kind: JobList
  version: v1

Ü
io.k8s.api.batch.v1.JobSpecÊ
„∫template object˙À
-
activeDeadlineSeconds
 integeröint64
$
backoffLimit
 integeröint32
,
backoffLimitPerIndex
 integeröint32
:
completionMode(
&¬
Indexed
¬NonIndexed
 string
#
completions
 integeröint32

	managedBy
	 string

manualSelector

 boolean
(
maxFailedIndexes
 integeröint32
#
parallelism
 integeröint32
Q
podFailurePolicy=;
9#/components/schemas/io.k8s.api.batch.v1.PodFailurePolicy
H
podReplacementPolicy0
.¬	Failed
¬TerminatingOrFailed
 string
W
selectorKI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.LabelSelector
K
successPolicy:8
6#/components/schemas/io.k8s.api.batch.v1.SuccessPolicy

suspend

 boolean
O
templateC
A“;9
7#/components/schemas/io.k8s.api.core.v1.PodTemplateSpecä 
/
ttlSecondsAfterFinished
 integeröint32
˝
io.k8s.api.batch.v1.JobStatus€
ÿ object˙À

active
 integeröint32

completedIndexes
	 string
T
completionTimeB@
>#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Time
⁄

conditionsÀ
» arrayÚC
A
?“97
5#/components/schemas/io.k8s.api.batch.v1.JobConditionä ¢#
x-kubernetes-list-type	atomic
¢'
x-kubernetes-patch-merge-keytype
¢'
x-kubernetes-patch-strategymerge


failed
 integeröint32

failedIndexes
	 string

ready
 integeröint32
O
	startTimeB@
>#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Time
!
	succeeded
 integeröint32
#
terminating
 integeröint32
_
uncountedTerminatedPodsDB
@#/components/schemas/io.k8s.api.batch.v1.UncountedTerminatedPods
‹
#io.k8s.api.batch.v1.JobTemplateSpec¥
± object˙§
\
metadataP
N“HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMetaä 
D
spec<
:“42
0#/components/schemas/io.k8s.api.batch.v1.JobSpecä 
À
$io.k8s.api.batch.v1.PodFailurePolicy¢
ü∫rules object˙ä
á
rules~
| arrayÚK
I
G“A?
=#/components/schemas/io.k8s.api.batch.v1.PodFailurePolicyRuleä ¢#
x-kubernetes-list-type	atomic

ê
:io.k8s.api.batch.v1.PodFailurePolicyOnExitCodesRequirement—
Œ∫operator∫values object˙≠

containerName
	 string
/
operator#
!¬In
¬NotIn
 stringä 
\
valuesR
P arrayÚ"
 
 integerä		        öint32¢ 
x-kubernetes-list-typeset

â
:io.k8s.api.batch.v1.PodFailurePolicyOnPodConditionsPatternK
I∫type object˙6

status
 stringä 

type
 stringä 
°
(io.k8s.api.batch.v1.PodFailurePolicyRuleÙ
Ò∫action object˙€
M
actionC
A¬Count
¬
FailIndex
¬
FailJob
¬	Ignore
 stringä 
b
onExitCodesSQ
O#/components/schemas/io.k8s.api.batch.v1.PodFailurePolicyOnExitCodesRequirement
•
onPodConditionsë
é arrayÚ]
[
Y“SQ
O#/components/schemas/io.k8s.api.batch.v1.PodFailurePolicyOnPodConditionsPatternä ¢#
x-kubernetes-list-type	atomic

≈
!io.k8s.api.batch.v1.SuccessPolicyü
ú∫rules object˙á
Ñ
rules{
y arrayÚH
F
D“><
:#/components/schemas/io.k8s.api.batch.v1.SuccessPolicyRuleä ¢#
x-kubernetes-list-type	atomic

Ä
%io.k8s.api.batch.v1.SuccessPolicyRuleW
U object˙I
&
succeededCount
 integeröint32

succeededIndexes
	 string
ﬂ
+io.k8s.api.batch.v1.UncountedTerminatedPodsØ
¨ object˙ü
L
failedB
@ arrayÚ

 stringä ¢ 
x-kubernetes-list-typeset

O
	succeededB
@ arrayÚ

 stringä ¢ 
x-kubernetes-list-typeset

ƒ
3io.k8s.api.core.v1.AWSElasticBlockStoreVolumeSourceå
â∫volumeID object˙r

fsType
	 string
!
	partition
 integeröint32

readOnly

 boolean

volumeID
 stringä 
í
io.k8s.api.core.v1.AffinityÚ
Ô object˙‚
H
nodeAffinity86
4#/components/schemas/io.k8s.api.core.v1.NodeAffinity
F
podAffinity75
3#/components/schemas/io.k8s.api.core.v1.PodAffinity
N
podAntiAffinity;9
7#/components/schemas/io.k8s.api.core.v1.PodAntiAffinity
†
"io.k8s.api.core.v1.AppArmorProfile˘
ˆ∫type object˙n

localhostProfile
	 string
K
typeC
A¬
Localhost
¬RuntimeDefault
¬Unconfined
 stringä ¢r
x-kubernetes-unions[Y- discriminator: type
  fields-to-discriminateBy:
    localhostProfile: LocalhostProfile

Â
(io.k8s.api.core.v1.AzureDiskVolumeSource∏
µ∫diskName∫diskURI object˙ì
O
cachingMode@
>¬None
¬	ReadOnly
¬
ReadWrite
 stringä	ReadWrite

diskName
 stringä 

diskURI
 stringä 

fsType
 stringäext4
F
kind>
<¬
Dedicated
¬
Managed
¬	Shared
 stringäShared

readOnly
 booleanä 
≠
(io.k8s.api.core.v1.AzureFileVolumeSourceÄ
~∫
secretName∫	shareName object˙Y

readOnly

 boolean


secretName
 stringä 

	shareName
 stringä 
ù
"io.k8s.api.core.v1.CSIVolumeSourceˆ
Û∫driver object˙›

driver
 stringä 

fsType
	 string
X
nodePublishSecretRef@>
<#/components/schemas/io.k8s.api.core.v1.LocalObjectReference

readOnly

 boolean
4
volumeAttributes 
 objectÇ

 stringä 
—
io.k8s.api.core.v1.Capabilities≠
™ object˙ù
L
addE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

M
dropE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

∆
%io.k8s.api.core.v1.CephFSVolumeSourceú
ô∫monitors object˙Å
Q
monitorsE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic


path
	 string

readOnly

 boolean


secretFile
	 string
M
	secretRef@>
<#/components/schemas/io.k8s.api.core.v1.LocalObjectReference

user
	 string
„
%io.k8s.api.core.v1.CinderVolumeSourceπ
∂∫volumeID object˙û

fsType
	 string

readOnly

 boolean
M
	secretRef@>
<#/components/schemas/io.k8s.api.core.v1.LocalObjectReference

volumeID
 stringä 
ç
/io.k8s.api.core.v1.ClusterTrustBundleProjectionŸ
÷∫path object˙¬
\
labelSelectorKI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.LabelSelector

name
	 string

optional

 boolean

path
 stringä 


signerName
	 string
k
%io.k8s.api.core.v1.ConfigMapEnvSourceB
@ object˙4

name
 stringä 

optional

 boolean
≥
'io.k8s.api.core.v1.ConfigMapKeySelectorá
Ñ∫key object˙M

key
 stringä 

name
 stringä 

optional

 boolean¢"
x-kubernetes-map-type	atomic

Ï
&io.k8s.api.core.v1.ConfigMapProjection¡
æ object˙±
{
itemsr
p arrayÚ?
=
;“53
1#/components/schemas/io.k8s.api.core.v1.KeyToPathä ¢#
x-kubernetes-list-type	atomic


name
 stringä 

optional

 boolean
ì
(io.k8s.api.core.v1.ConfigMapVolumeSourceÊ
„ object˙÷
#
defaultMode
 integeröint32
{
itemsr
p arrayÚ?
=
;“53
1#/components/schemas/io.k8s.api.core.v1.KeyToPathä ¢#
x-kubernetes-list-type	atomic


name
 stringä 

optional

 boolean
€
io.k8s.api.core.v1.Container∫
∑∫name object˙£
M
argsE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

P
commandE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

Û
envÎ
Ë arrayÚ<
:
8“20
.#/components/schemas/io.k8s.api.core.v1.EnvVarä ¢'
x-kubernetes-list-map-keys	- name
¢ 
x-kubernetes-list-typemap
¢'
x-kubernetes-patch-merge-keyname
¢'
x-kubernetes-patch-strategymerge

Å
envFromv
t arrayÚC
A
?“97
5#/components/schemas/io.k8s.api.core.v1.EnvFromSourceä ¢#
x-kubernetes-list-type	atomic


image
	 string
G
imagePullPolicy4
2¬	Always
¬IfNotPresent
¬Never
 string
B
	lifecycle53
1#/components/schemas/io.k8s.api.core.v1.Lifecycle
B
livenessProbe1/
-#/components/schemas/io.k8s.api.core.v1.Probe

name
 stringä 
ô
portsè
å arrayÚC
A
?“97
5#/components/schemas/io.k8s.api.core.v1.ContainerPortä ¢;
x-kubernetes-list-map-keys- containerPort
- protocol
¢ 
x-kubernetes-list-typemap
¢0
x-kubernetes-patch-merge-keycontainerPort
¢'
x-kubernetes-patch-strategymerge

C
readinessProbe1/
-#/components/schemas/io.k8s.api.core.v1.Probe
é
resizePolicy~
| arrayÚK
I
G“A?
=#/components/schemas/io.k8s.api.core.v1.ContainerResizePolicyä ¢#
x-kubernetes-list-type	atomic

U
	resourcesH
F“@>
<#/components/schemas/io.k8s.api.core.v1.ResourceRequirementsä 

restartPolicy
	 string
ì
restartPolicyRules}
{ arrayÚJ
H
F“@>
<#/components/schemas/io.k8s.api.core.v1.ContainerRestartRuleä ¢#
x-kubernetes-list-type	atomic

N
securityContext;9
7#/components/schemas/io.k8s.api.core.v1.SecurityContext
A
startupProbe1/
-#/components/schemas/io.k8s.api.core.v1.Probe

stdin

 boolean

	stdinOnce

 boolean
%
terminationMessagePath
	 string
L
terminationMessagePolicy0
.¬FallbackToLogsOnError
¬File
 string

tty

 boolean
è
volumeDevices˝
˙ arrayÚB
@
>“86
4#/components/schemas/io.k8s.api.core.v1.VolumeDeviceä ¢-
x-kubernetes-list-map-keys- devicePath
¢ 
x-kubernetes-list-typemap
¢-
x-kubernetes-patch-merge-keydevicePath
¢'
x-kubernetes-patch-strategymerge

ã
volumeMounts˙
˜ arrayÚA
?
=“75
3#/components/schemas/io.k8s.api.core.v1.VolumeMountä ¢,
x-kubernetes-list-map-keys- mountPath
¢ 
x-kubernetes-list-typemap
¢,
x-kubernetes-patch-merge-key
mountPath
¢'
x-kubernetes-patch-strategymerge



workingDir
	 string
É
 io.k8s.api.core.v1.ContainerPortﬁ
€∫containerPort object˙æ
1
containerPort 
 integerä		        öint32

hostIP
	 string
 
hostPort
 integeröint32

name
	 string
;
protocol/
-¬SCTP
¬TCP
¬UDP
 stringäTCP
û
(io.k8s.api.core.v1.ContainerResizePolicyr
p∫resourceName∫restartPolicy object˙E
 
resourceName
 stringä 
!
restartPolicy
 stringä 
µ
'io.k8s.api.core.v1.ContainerRestartRuleâ
Ü∫action object˙q

action
	 string
X
	exitCodesKI
G#/components/schemas/io.k8s.api.core.v1.ContainerRestartRuleOnExitCodes
»
2io.k8s.api.core.v1.ContainerRestartRuleOnExitCodesë
é∫operator object˙w

operator
	 string
\
valuesR
P arrayÚ"
 
 integerä		        öint32¢ 
x-kubernetes-list-typeset

«
(io.k8s.api.core.v1.DownwardAPIProjectionö
ó object˙ä
á
items~
| arrayÚK
I
G“A?
=#/components/schemas/io.k8s.api.core.v1.DownwardAPIVolumeFileä ¢#
x-kubernetes-list-type	atomic

†
(io.k8s.api.core.v1.DownwardAPIVolumeFileÛ
∫path object˙‹
K
fieldRef?=
;#/components/schemas/io.k8s.api.core.v1.ObjectFieldSelector

mode
 integeröint32

path
 stringä 
U
resourceFieldRefA?
=#/components/schemas/io.k8s.api.core.v1.ResourceFieldSelector
Ó
*io.k8s.api.core.v1.DownwardAPIVolumeSourceø
º object˙Ø
#
defaultMode
 integeröint32
á
items~
| arrayÚK
I
G“A?
=#/components/schemas/io.k8s.api.core.v1.DownwardAPIVolumeFileä ¢#
x-kubernetes-list-type	atomic

•
'io.k8s.api.core.v1.EmptyDirVolumeSourcez
x object˙l

medium
	 string
S
	sizeLimitFD
B#/components/schemas/io.k8s.apimachinery.pkg.api.resource.Quantity
Ê
 io.k8s.api.core.v1.EnvFromSource¡
æ object˙±
N
configMapRef><
:#/components/schemas/io.k8s.api.core.v1.ConfigMapEnvSource

prefix
	 string
H
	secretRef;9
7#/components/schemas/io.k8s.api.core.v1.SecretEnvSource
´
io.k8s.api.core.v1.EnvVarç
ä∫name object˙w

name
 stringä 

value
	 string
E
	valueFrom86
4#/components/schemas/io.k8s.api.core.v1.EnvVarSource
«
io.k8s.api.core.v1.EnvVarSource£
† object˙ì
S
configMapKeyRef@>
<#/components/schemas/io.k8s.api.core.v1.ConfigMapKeySelector
K
fieldRef?=
;#/components/schemas/io.k8s.api.core.v1.ObjectFieldSelector
I

fileKeyRef;9
7#/components/schemas/io.k8s.api.core.v1.FileKeySelector
U
resourceFieldRefA?
=#/components/schemas/io.k8s.api.core.v1.ResourceFieldSelector
M
secretKeyRef=;
9#/components/schemas/io.k8s.api.core.v1.SecretKeySelector
à
%io.k8s.api.core.v1.EphemeralContainerﬁ
€∫name object˙«
M
argsE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

P
commandE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

Û
envÎ
Ë arrayÚ<
:
8“20
.#/components/schemas/io.k8s.api.core.v1.EnvVarä ¢'
x-kubernetes-list-map-keys	- name
¢ 
x-kubernetes-list-typemap
¢'
x-kubernetes-patch-merge-keyname
¢'
x-kubernetes-patch-strategymerge

Å
envFromv
t arrayÚC
A
?“97
5#/components/schemas/io.k8s.api.core.v1.EnvFromSourceä ¢#
x-kubernetes-list-type	atomic


image
	 string
G
imagePullPolicy4
2¬	Always
¬IfNotPresent
¬Never
 string
B
	lifecycle53
1#/components/schemas/io.k8s.api.core.v1.Lifecycle
B
livenessProbe1/
-#/components/schemas/io.k8s.api.core.v1.Probe

name
 stringä 
ô
portsè
å arrayÚC
A
?“97
5#/components/schemas/io.k8s.api.core.v1.ContainerPortä ¢;
x-kubernetes-list-map-keys- containerPort
- protocol
¢ 
x-kubernetes-list-typemap
¢0
x-kubernetes-patch-merge-keycontainerPort
¢'
x-kubernetes-patch-strategymerge

C
readinessProbe1/
-#/components/schemas/io.k8s.api.core.v1.Probe
é
resizePolicy~
| arrayÚK
I
G“A?
=#/components/schemas/io.k8s.api.core.v1.ContainerResizePolicyä ¢#
x-kubernetes-list-type	atomic

U
	resourcesH
F“@>
<#/components/schemas/io.k8s.api.core.v1.ResourceRequirementsä 

restartPolicy
	 string
ì
restartPolicyRules}
{ arrayÚJ
H
F“@>
<#/components/schemas/io.k8s.api.core.v1.ContainerRestartRuleä ¢#
x-kubernetes-list-type	atomic

N
securityContext;9
7#/components/schemas/io.k8s.api.core.v1.SecurityContext
A
startupProbe1/
-#/components/schemas/io.k8s.api.core.v1.Probe

stdin

 boolean

	stdinOnce

 boolean
"
targetContainerName
	 string
%
terminationMessagePath
	 string
L
terminationMessagePolicy0
.¬FallbackToLogsOnError
¬File
 string

tty

 boolean
è
volumeDevices˝
˙ arrayÚB
@
>“86
4#/components/schemas/io.k8s.api.core.v1.VolumeDeviceä ¢-
x-kubernetes-list-map-keys- devicePath
¢ 
x-kubernetes-list-typemap
¢-
x-kubernetes-patch-merge-keydevicePath
¢'
x-kubernetes-patch-strategymerge

ã
volumeMounts˙
˜ arrayÚA
?
=“75
3#/components/schemas/io.k8s.api.core.v1.VolumeMountä ¢,
x-kubernetes-list-map-keys- mountPath
¢ 
x-kubernetes-list-typemap
¢,
x-kubernetes-patch-merge-key
mountPath
¢'
x-kubernetes-patch-strategymerge



workingDir
	 string
ú
(io.k8s.api.core.v1.EphemeralVolumeSourcep
n object˙b
`
volumeClaimTemplateIG
E#/components/schemas/io.k8s.api.core.v1.PersistentVolumeClaimTemplate
Å
io.k8s.api.core.v1.ExecAction`
^ object˙R
P
commandE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

©
!io.k8s.api.core.v1.FCVolumeSourceÉ
Ä object˙Û

fsType
	 string

lun
 integeröint32

readOnly

 boolean
S

targetWWNsE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

N
wwidsE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

Á
"io.k8s.api.core.v1.FileKeySelector¿
Ω∫
volumeName∫path∫key object˙r

key
 stringä 

optional
 booleanä 

path
 stringä 


volumeName
 stringä ¢"
x-kubernetes-map-type	atomic

ä
#io.k8s.api.core.v1.FlexVolumeSource‚
ﬂ∫driver object˙…

driver
 stringä 

fsType
	 string
+
options 
 objectÇ

 stringä 

readOnly

 boolean
M
	secretRef@>
<#/components/schemas/io.k8s.api.core.v1.LocalObjectReference
p
&io.k8s.api.core.v1.FlockerVolumeSourceF
D object˙8

datasetName
	 string

datasetUUID
	 string
Ω
0io.k8s.api.core.v1.GCEPersistentDiskVolumeSourceà
Ö∫pdName object˙p

fsType
	 string
!
	partition
 integeröint32

pdName
 stringä 

readOnly

 boolean
}
io.k8s.api.core.v1.GRPCAction\
Z∫port object˙G
(
port 
 integerä		        öint32

service
 stringä 
ò
&io.k8s.api.core.v1.GitRepoVolumeSourcen
l∫
repository object˙S

	directory
	 string


repository
 stringä 

revision
	 string
†
(io.k8s.api.core.v1.GlusterfsVolumeSourcet
r∫	endpoints∫path object˙S

	endpoints
 stringä 

path
 stringä 

readOnly

 boolean
È
 io.k8s.api.core.v1.HTTPGetActionƒ
¡∫port object˙≠

host
	 string
Ç
httpHeaderss
q arrayÚ@
>
<“64
2#/components/schemas/io.k8s.api.core.v1.HTTPHeaderä ¢#
x-kubernetes-list-type	atomic


path
	 string
P
portHF
D#/components/schemas/io.k8s.apimachinery.pkg.util.intstr.IntOrString
*
scheme 
¬HTTP
¬HTTPS
 string
s
io.k8s.api.core.v1.HTTPHeaderR
P∫name∫value object˙5

name
 stringä 

value
 stringä 
ü
io.k8s.api.core.v1.HostAlias
}∫ip object˙l
R
	hostnamesE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic


ip
 stringä 
Ï
'io.k8s.api.core.v1.HostPathVolumeSource¿
Ω∫path object˙©

path
 stringä 
å
typeÉ
Ä¬""
¬BlockDevice
¬CharDevice
¬
Directory
¬DirectoryOrCreate
¬File
¬FileOrCreate
¬	Socket
 string
ó
$io.k8s.api.core.v1.ISCSIVolumeSourceÓ
Î∫targetPortal∫iqn∫lun object˙√
!
chapAuthDiscovery

 boolean

chapAuthSession

 boolean

fsType
	 string

initiatorName
	 string

iqn
 stringä 
)
iscsiInterface
 stringä	default
'
lun 
 integerä		        öint32
P
portalsE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic


readOnly

 boolean
M
	secretRef@>
<#/components/schemas/io.k8s.api.core.v1.LocalObjectReference
 
targetPortal
 stringä 
î
$io.k8s.api.core.v1.ImageVolumeSourcel
j object˙^
B

pullPolicy4
2¬	Always
¬IfNotPresent
¬Never
 string

	reference
	 string
å
io.k8s.api.core.v1.KeyToPathl
j∫key∫path object˙Q

key
 stringä 

mode
 integeröint32

path
 stringä 
ç	
io.k8s.api.core.v1.LifecycleÏ
È object˙‹
I
	postStart<:
8#/components/schemas/io.k8s.api.core.v1.LifecycleHandler
G
preStop<:
8#/components/schemas/io.k8s.api.core.v1.LifecycleHandler
≈

stopSignal∂
≥¬
SIGABRT
¬
SIGALRM
¬	SIGBUS
¬
SIGCHLD
¬	SIGCLD
¬
SIGCONT
¬	SIGFPE
¬	SIGHUP
¬	SIGILL
¬	SIGINT
¬SIGIO
¬	SIGIOT
¬
SIGKILL
¬
SIGPIPE
¬
SIGPOLL
¬
SIGPROF
¬	SIGPWR
¬
SIGQUIT
¬	SIGRTMAX
¬SIGRTMAX-1
¬SIGRTMAX-10
¬SIGRTMAX-11
¬SIGRTMAX-12
¬SIGRTMAX-13
¬SIGRTMAX-14
¬SIGRTMAX-2
¬SIGRTMAX-3
¬SIGRTMAX-4
¬SIGRTMAX-5
¬SIGRTMAX-6
¬SIGRTMAX-7
¬SIGRTMAX-8
¬SIGRTMAX-9
¬	SIGRTMIN
¬SIGRTMIN+1
¬SIGRTMIN+10
¬SIGRTMIN+11
¬SIGRTMIN+12
¬SIGRTMIN+13
¬SIGRTMIN+14
¬SIGRTMIN+15
¬SIGRTMIN+2
¬SIGRTMIN+3
¬SIGRTMIN+4
¬SIGRTMIN+5
¬SIGRTMIN+6
¬SIGRTMIN+7
¬SIGRTMIN+8
¬SIGRTMIN+9
¬
SIGSEGV
¬
SIGSTKFLT
¬
SIGSTOP
¬	SIGSYS
¬
SIGTERM
¬
SIGTRAP
¬
SIGTSTP
¬
SIGTTIN
¬
SIGTTOU
¬	SIGURG
¬
SIGUSR1
¬
SIGUSR2
¬
SIGVTALRM
¬	SIGWINCH
¬
SIGXCPU
¬
SIGXFSZ
 string
 
#io.k8s.api.core.v1.LifecycleHandler¢
ü object˙í
>
exec64
2#/components/schemas/io.k8s.api.core.v1.ExecAction
D
httpGet97
5#/components/schemas/io.k8s.api.core.v1.HTTPGetAction
@
sleep75
3#/components/schemas/io.k8s.api.core.v1.SleepAction
H
	tcpSocket;9
7#/components/schemas/io.k8s.api.core.v1.TCPSocketAction
x
'io.k8s.api.core.v1.LocalObjectReferenceM
K object˙

name
 stringä ¢"
x-kubernetes-map-type	atomic

î
"io.k8s.api.core.v1.NFSVolumeSourcen
l∫server∫path object˙P

path
 stringä 

readOnly

 boolean

server
 stringä 
◊
io.k8s.api.core.v1.NodeAffinity≥
∞ object˙£
¥
/preferredDuringSchedulingIgnoredDuringExecutionÄ
~ arrayÚM
K
I“CA
?#/components/schemas/io.k8s.api.core.v1.PreferredSchedulingTermä ¢#
x-kubernetes-list-type	atomic

j
.requiredDuringSchedulingIgnoredDuringExecution86
4#/components/schemas/io.k8s.api.core.v1.NodeSelector
˛
io.k8s.api.core.v1.NodeSelector⁄
◊∫nodeSelectorTerms object˙ë
é
nodeSelectorTermsy
w arrayÚF
D
B“<:
8#/components/schemas/io.k8s.api.core.v1.NodeSelectorTermä ¢#
x-kubernetes-list-type	atomic
¢"
x-kubernetes-map-type	atomic

ô
*io.k8s.api.core.v1.NodeSelectorRequirementÍ
Á∫key∫operator object˙…

key
 stringä 
]
operatorQ
O¬DoesNotExist
¬	Exists
¬Gt
¬In
¬Lt
¬NotIn
 stringä 
O
valuesE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

à
#io.k8s.api.core.v1.NodeSelectorTerm‡
› object˙´
ï
matchExpressionsÄ
~ arrayÚM
K
I“CA
?#/components/schemas/io.k8s.api.core.v1.NodeSelectorRequirementä ¢#
x-kubernetes-list-type	atomic

ê
matchFieldsÄ
~ arrayÚM
K
I“CA
?#/components/schemas/io.k8s.api.core.v1.NodeSelectorRequirementä ¢#
x-kubernetes-list-type	atomic
¢"
x-kubernetes-map-type	atomic

£
&io.k8s.api.core.v1.ObjectFieldSelectory
w∫	fieldPath object˙:


apiVersion
	 string

	fieldPath
 stringä ¢"
x-kubernetes-map-type	atomic

â
"io.k8s.api.core.v1.ObjectReference‚
ﬂ object˙≠


apiVersion
	 string

	fieldPath
	 string

kind
	 string

name
	 string

	namespace
	 string

resourceVersion
	 string

uid
	 string¢"
x-kubernetes-map-type	atomic

‚
,io.k8s.api.core.v1.PersistentVolumeClaimSpec±
Æ object˙°
§
accessModesî
ë arrayÚ`
^
\¬ReadOnlyMany
¬ReadWriteMany
¬ReadWriteOnce
¬ReadWriteOncePod
 stringä ¢#
x-kubernetes-list-type	atomic

S

dataSourceEC
A#/components/schemas/io.k8s.api.core.v1.TypedLocalObjectReference
Q
dataSourceRef@>
<#/components/schemas/io.k8s.api.core.v1.TypedObjectReference
[
	resourcesN
L“FD
B#/components/schemas/io.k8s.api.core.v1.VolumeResourceRequirementsä 
W
selectorKI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.LabelSelector

storageClassName
	 string
(
volumeAttributesClassName
	 string
4

volumeMode&
$¬Block
¬Filesystem
 string


volumeName
	 string
Å
0io.k8s.api.core.v1.PersistentVolumeClaimTemplateÃ
…∫spec object˙µ
\
metadataP
N“HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMetaä 
U
specM
K“EC
A#/components/schemas/io.k8s.api.core.v1.PersistentVolumeClaimSpecä 
ã
4io.k8s.api.core.v1.PersistentVolumeClaimVolumeSourceS
Q∫	claimName object˙9

	claimName
 stringä 

readOnly

 boolean
}
3io.k8s.api.core.v1.PhotonPersistentDiskVolumeSourceF
D∫pdID object˙1

fsType
	 string

pdID
 stringä 
ó
io.k8s.api.core.v1.PodAffinityÙ
Ò object˙‰
¥
/preferredDuringSchedulingIgnoredDuringExecutionÄ
~ arrayÚM
K
I“CA
?#/components/schemas/io.k8s.api.core.v1.WeightedPodAffinityTermä ¢#
x-kubernetes-list-type	atomic

™
.requiredDuringSchedulingIgnoredDuringExecutionx
v arrayÚE
C
A“;9
7#/components/schemas/io.k8s.api.core.v1.PodAffinityTermä ¢#
x-kubernetes-list-type	atomic

∞
"io.k8s.api.core.v1.PodAffinityTermâ
Ü∫topologyKey object˙Î
\
labelSelectorKI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.LabelSelector
W
matchLabelKeysE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

Z
mismatchLabelKeysE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

`
namespaceSelectorKI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.LabelSelector
S

namespacesE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic


topologyKey
 stringä 
õ
"io.k8s.api.core.v1.PodAntiAffinityÙ
Ò object˙‰
¥
/preferredDuringSchedulingIgnoredDuringExecutionÄ
~ arrayÚM
K
I“CA
?#/components/schemas/io.k8s.api.core.v1.WeightedPodAffinityTermä ¢#
x-kubernetes-list-type	atomic

™
.requiredDuringSchedulingIgnoredDuringExecutionx
v arrayÚE
C
A“;9
7#/components/schemas/io.k8s.api.core.v1.PodAffinityTermä ¢#
x-kubernetes-list-type	atomic

œ
+io.k8s.api.core.v1.PodCertificateProjectionü
ú∫
signerName∫keyType object˙¯
#
certificateChainPath
	 string
#
credentialBundlePath
	 string

keyPath
	 string

keyType
	 string
,
maxExpirationSeconds
 integeröint32


signerName
	 string
3
userAnnotations 
 objectÇ

 stringä 
Ê
io.k8s.api.core.v1.PodDNSConfig¬
ø object˙≤
T
nameserversE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

Ü
options{
y arrayÚH
F
D“><
:#/components/schemas/io.k8s.api.core.v1.PodDNSConfigOptionä ¢#
x-kubernetes-list-type	atomic

Q
searchesE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

b
%io.k8s.api.core.v1.PodDNSConfigOption9
7 object˙+

name
	 string

value
	 string
K
io.k8s.api.core.v1.PodOS/
-∫name object˙

name
 stringä 
h
#io.k8s.api.core.v1.PodReadinessGateA
?∫conditionType object˙#
!
conditionType
 stringä 
¢
#io.k8s.api.core.v1.PodResourceClaim{
y∫name object˙f

name
 stringä 
 
resourceClaimName
	 string
(
resourceClaimTemplateName
	 string
W
$io.k8s.api.core.v1.PodSchedulingGate/
-∫name object˙

name
 stringä 
≠
%io.k8s.api.core.v1.PodSchedulingGroupÉ
Ä object˙

podGroupName
	 string¢T
x-kubernetes-unions=;- fields-to-discriminateBy:
    podGroupName: PodGroupName

ö
%io.k8s.api.core.v1.PodSecurityContext
Ì object˙‡
N
appArmorProfile;9
7#/components/schemas/io.k8s.api.core.v1.AppArmorProfile

fsGroup
 integeröint64
B
fsGroupChangePolicy+
)¬	Always
¬OnRootMismatch
 string
"

runAsGroup
 integeröint64

runAsNonRoot

 boolean
!
	runAsUser
 integeröint64
"
seLinuxChangePolicy
	 string
L
seLinuxOptions:8
6#/components/schemas/io.k8s.api.core.v1.SELinuxOptions
L
seccompProfile:8
6#/components/schemas/io.k8s.api.core.v1.SeccompProfile
k
supplementalGroupsU
S arrayÚ"
 
 integerä		        öint64¢#
x-kubernetes-list-type	atomic

>
supplementalGroupsPolicy"
 ¬Merge
¬	Strict
 string
z
sysctlso
m arrayÚ<
:
8“20
.#/components/schemas/io.k8s.api.core.v1.Sysctlä ¢#
x-kubernetes-list-type	atomic

[
windowsOptionsIG
E#/components/schemas/io.k8s.api.core.v1.WindowsSecurityContextOptions
’!
io.k8s.api.core.v1.PodSpec∂!
≥!∫
containers object˙ô!
-
activeDeadlineSeconds
 integeröint64
@
affinity42
0#/components/schemas/io.k8s.api.core.v1.Affinity
,
automountServiceAccountToken

 boolean
˝

containersÓ
Î arrayÚ?
=
;“53
1#/components/schemas/io.k8s.api.core.v1.Containerä ¢'
x-kubernetes-list-map-keys	- name
¢ 
x-kubernetes-list-typemap
¢'
x-kubernetes-patch-merge-keyname
¢'
x-kubernetes-patch-strategymerge

E
	dnsConfig86
4#/components/schemas/io.k8s.api.core.v1.PodDNSConfig
^
	dnsPolicyQ
O¬ClusterFirst
¬ClusterFirstWithHostNet
¬
Default
¬None
 string
"
enableServiceLinks

 boolean
è
ephemeralContainers˜
Ù arrayÚH
F
D“><
:#/components/schemas/io.k8s.api.core.v1.EphemeralContainerä ¢'
x-kubernetes-list-map-keys	- name
¢ 
x-kubernetes-list-typemap
¢'
x-kubernetes-patch-merge-keyname
¢'
x-kubernetes-patch-strategymerge

˙
hostAliasesÍ
Á arrayÚ?
=
;“53
1#/components/schemas/io.k8s.api.core.v1.HostAliasä ¢%
x-kubernetes-list-map-keys- ip
¢ 
x-kubernetes-list-typemap
¢%
x-kubernetes-patch-merge-keyip
¢'
x-kubernetes-patch-strategymerge


hostIPC

 boolean

hostNetwork

 boolean

hostPID

 boolean

	hostUsers

 boolean

hostname
	 string

hostnameOverride
	 string
é
imagePullSecrets˘
ˆ arrayÚJ
H
F“@>
<#/components/schemas/io.k8s.api.core.v1.LocalObjectReferenceä ¢'
x-kubernetes-list-map-keys	- name
¢ 
x-kubernetes-list-typemap
¢'
x-kubernetes-patch-merge-keyname
¢'
x-kubernetes-patch-strategymerge

Å
initContainersÓ
Î arrayÚ?
=
;“53
1#/components/schemas/io.k8s.api.core.v1.Containerä ¢'
x-kubernetes-list-map-keys	- name
¢ 
x-kubernetes-list-typemap
¢'
x-kubernetes-patch-merge-keyname
¢'
x-kubernetes-patch-strategymerge


nodeName
	 string
U
nodeSelectorE
C objectÇ

 stringä ¢"
x-kubernetes-map-type	atomic

7
os1/
-#/components/schemas/io.k8s.api.core.v1.PodOS
b
overheadV
T objectÇH
FD
B#/components/schemas/io.k8s.apimachinery.pkg.api.resource.Quantity
D
preemptionPolicy0
.¬Never
¬PreemptLowerPriority
 string
 
priority
 integeröint32
 
priorityClassName
	 string
ã
readinessGatesy
w arrayÚF
D
B“<:
8#/components/schemas/io.k8s.api.core.v1.PodReadinessGateä ¢#
x-kubernetes-list-type	atomic

ì
resourceClaimsÄ
˝ arrayÚF
D
B“<:
8#/components/schemas/io.k8s.api.core.v1.PodResourceClaimä ¢'
x-kubernetes-list-map-keys	- name
¢ 
x-kubernetes-list-typemap
¢'
x-kubernetes-patch-merge-keyname
¢2
x-kubernetes-patch-strategymerge,retainKeys

M
	resources@>
<#/components/schemas/io.k8s.api.core.v1.ResourceRequirements
B
restartPolicy1
/¬	Always
¬Never
¬
OnFailure
 string

runtimeClassName
	 string

schedulerName
	 string
ä
schedulingGatesˆ
Û arrayÚG
E
C“=;
9#/components/schemas/io.k8s.api.core.v1.PodSchedulingGateä ¢'
x-kubernetes-list-map-keys	- name
¢ 
x-kubernetes-list-typemap
¢'
x-kubernetes-patch-merge-keyname
¢'
x-kubernetes-patch-strategymerge

Q
schedulingGroup><
:#/components/schemas/io.k8s.api.core.v1.PodSchedulingGroup
Q
securityContext><
:#/components/schemas/io.k8s.api.core.v1.PodSecurityContext

serviceAccount
	 string
!
serviceAccountName
	 string
!
setHostnameAsFQDN

 boolean
%
shareProcessNamespace

 boolean

	subdomain
	 string
5
terminationGracePeriodSeconds
 integeröint64
Ç
tolerationss
q arrayÚ@
>
<“64
2#/components/schemas/io.k8s.api.core.v1.Tolerationä ¢#
x-kubernetes-list-type	atomic

Ω
topologySpreadConstraintsü
ú arrayÚN
L
J“DB
@#/components/schemas/io.k8s.api.core.v1.TopologySpreadConstraintä ¢B
x-kubernetes-list-map-keys$"- topologyKey
- whenUnsatisfiable
¢ 
x-kubernetes-list-typemap
¢.
x-kubernetes-patch-merge-keytopologyKey
¢'
x-kubernetes-patch-strategymerge

Ç
volumesˆ
Û arrayÚ<
:
8“20
.#/components/schemas/io.k8s.api.core.v1.Volumeä ¢'
x-kubernetes-list-map-keys	- name
¢ 
x-kubernetes-list-typemap
¢'
x-kubernetes-patch-merge-keyname
¢2
x-kubernetes-patch-strategymerge,retainKeys

⁄
"io.k8s.api.core.v1.PodTemplateSpec≥
∞ object˙£
\
metadataP
N“HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMetaä 
C
spec;
9“31
/#/components/schemas/io.k8s.api.core.v1.PodSpecä 
ì
'io.k8s.api.core.v1.PortworxVolumeSourceh
f∫volumeID object˙O

fsType
	 string

readOnly

 boolean

volumeID
 stringä 
’
*io.k8s.api.core.v1.PreferredSchedulingTerm¶
£∫weight∫
preference object˙Ä
R

preferenceD
B“<:
8#/components/schemas/io.k8s.api.core.v1.NodeSelectorTermä 
*
weight 
 integerä		        öint32
ƒ
io.k8s.api.core.v1.Probeß
§ object˙ó
>
exec64
2#/components/schemas/io.k8s.api.core.v1.ExecAction
(
failureThreshold
 integeröint32
>
grpc64
2#/components/schemas/io.k8s.api.core.v1.GRPCAction
D
httpGet97
5#/components/schemas/io.k8s.api.core.v1.HTTPGetAction
+
initialDelaySeconds
 integeröint32
%
periodSeconds
 integeröint32
(
successThreshold
 integeröint32
H
	tcpSocket;9
7#/components/schemas/io.k8s.api.core.v1.TCPSocketAction
5
terminationGracePeriodSeconds
 integeröint64
&
timeoutSeconds
 integeröint32
È
(io.k8s.api.core.v1.ProjectedVolumeSourceº
π object˙¨
#
defaultMode
 integeröint32
Ñ
sourcesy
w arrayÚF
D
B“<:
8#/components/schemas/io.k8s.api.core.v1.VolumeProjectionä ¢#
x-kubernetes-list-type	atomic

Â
&io.k8s.api.core.v1.QuobyteVolumeSource∫
∑∫registry∫volume object˙ñ

group
	 string

readOnly

 boolean

registry
 stringä 

tenant
	 string

user
	 string

volume
 stringä 
¢
"io.k8s.api.core.v1.RBDVolumeSource˚
¯∫monitors∫image object˙ÿ

fsType
	 string

image
 stringä 
,
keyring!
 stringä/etc/ceph/keyring
Q
monitorsE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic


pool
 stringärbd

readOnly

 boolean
M
	secretRef@>
<#/components/schemas/io.k8s.api.core.v1.LocalObjectReference

user
 stringäadmin
k
 io.k8s.api.core.v1.ResourceClaimG
E∫name object˙2

name
 stringä 

request
	 string
¸
(io.k8s.api.core.v1.ResourceFieldSelectorœ
Ã∫resource object˙è

containerName
	 string
Q
divisorFD
B#/components/schemas/io.k8s.apimachinery.pkg.api.resource.Quantity

resource
 stringä ¢"
x-kubernetes-map-type	atomic

Æ
'io.k8s.api.core.v1.ResourceRequirementsÇ
ˇ object˙Ú
©
claimsû
õ arrayÚC
A
?“97
5#/components/schemas/io.k8s.api.core.v1.ResourceClaimä ¢'
x-kubernetes-list-map-keys	- name
¢ 
x-kubernetes-list-typemap

`
limitsV
T objectÇH
FD
B#/components/schemas/io.k8s.apimachinery.pkg.api.resource.Quantity
b
requestsV
T objectÇH
FD
B#/components/schemas/io.k8s.apimachinery.pkg.api.resource.Quantity
à
!io.k8s.api.core.v1.SELinuxOptionsc
a object˙U

level
	 string

role
	 string

type
	 string

user
	 string
ø
&io.k8s.api.core.v1.ScaleIOVolumeSourceî
ë∫gateway∫system∫	secretRef object˙Â

fsType
 stringäxfs

gateway
 stringä 

protectionDomain
	 string

readOnly

 boolean
M
	secretRef@>
<#/components/schemas/io.k8s.api.core.v1.LocalObjectReference


sslEnabled

 boolean
.
storageMode
 stringäThinProvisioned

storagePool
	 string

system
 stringä 


volumeName
	 string
ü
!io.k8s.api.core.v1.SeccompProfile˘
ˆ∫type object˙n

localhostProfile
	 string
K
typeC
A¬
Localhost
¬RuntimeDefault
¬Unconfined
 stringä ¢r
x-kubernetes-unions[Y- discriminator: type
  fields-to-discriminateBy:
    localhostProfile: LocalhostProfile

h
"io.k8s.api.core.v1.SecretEnvSourceB
@ object˙4

name
 stringä 

optional

 boolean
∞
$io.k8s.api.core.v1.SecretKeySelectorá
Ñ∫key object˙M

key
 stringä 

name
 stringä 

optional

 boolean¢"
x-kubernetes-map-type	atomic

È
#io.k8s.api.core.v1.SecretProjection¡
æ object˙±
{
itemsr
p arrayÚ?
=
;“53
1#/components/schemas/io.k8s.api.core.v1.KeyToPathä ¢#
x-kubernetes-list-type	atomic


name
 stringä 

optional

 boolean
ë
%io.k8s.api.core.v1.SecretVolumeSourceÁ
‰ object˙◊
#
defaultMode
 integeröint32
{
itemsr
p arrayÚ?
=
;“53
1#/components/schemas/io.k8s.api.core.v1.KeyToPathä ¢#
x-kubernetes-list-type	atomic


optional

 boolean


secretName
	 string
“
"io.k8s.api.core.v1.SecurityContext´
® object˙õ
(
allowPrivilegeEscalation

 boolean
N
appArmorProfile;9
7#/components/schemas/io.k8s.api.core.v1.AppArmorProfile
H
capabilities86
4#/components/schemas/io.k8s.api.core.v1.Capabilities


privileged

 boolean
3
	procMount&
$¬
Default
¬	Unmasked
 string
&
readOnlyRootFilesystem

 boolean
"

runAsGroup
 integeröint64

runAsNonRoot

 boolean
!
	runAsUser
 integeröint64
L
seLinuxOptions:8
6#/components/schemas/io.k8s.api.core.v1.SELinuxOptions
L
seccompProfile:8
6#/components/schemas/io.k8s.api.core.v1.SeccompProfile
[
windowsOptionsIG
E#/components/schemas/io.k8s.api.core.v1.WindowsSecurityContextOptions
ß
0io.k8s.api.core.v1.ServiceAccountTokenProjections
q∫path object˙^

audience
	 string
)
expirationSeconds
 integeröint64

path
 stringä 
g
io.k8s.api.core.v1.SleepActionE
C∫seconds object˙-
+
seconds 
 integerä		        öint64
¯
(io.k8s.api.core.v1.StorageOSVolumeSourceÀ
» object˙ª

fsType
	 string

readOnly

 boolean
M
	secretRef@>
<#/components/schemas/io.k8s.api.core.v1.LocalObjectReference


volumeName
	 string

volumeNamespace
	 string
o
io.k8s.api.core.v1.SysctlR
P∫name∫value object˙5

name
 stringä 

value
 stringä 
¢
"io.k8s.api.core.v1.TCPSocketAction|
z∫port object˙g

host
	 string
P
portHF
D#/components/schemas/io.k8s.apimachinery.pkg.util.intstr.IntOrString
ì
io.k8s.api.core.v1.TolerationÒ
Ó object˙·
J
effect@
>¬
NoExecute
¬NoSchedule
¬PreferNoSchedule
 string

key
	 string
>
operator2
0¬Equal
¬	Exists
¬Gt
¬Lt
 string
)
tolerationSeconds
 integeröint64

value
	 string
’
+io.k8s.api.core.v1.TopologySpreadConstraint•
¢∫maxSkew∫topologyKey∫whenUnsatisfiable object˙È
\
labelSelectorKI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.LabelSelector
W
matchLabelKeysE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

+
maxSkew 
 integerä		        öint32
"

minDomains
 integeröint32
8
nodeAffinityPolicy"
 ¬Honor
¬	Ignore
 string
6
nodeTaintsPolicy"
 ¬Honor
¬	Ignore
 string

topologyKey
 stringä 
L
whenUnsatisfiable7
5¬DoNotSchedule
¬ScheduleAnyway
 stringä 
¿
,io.k8s.api.core.v1.TypedLocalObjectReferenceè
å∫kind∫name object˙M

apiGroup
	 string

kind
 stringä 

name
 stringä ¢"
x-kubernetes-map-type	atomic

∞
'io.k8s.api.core.v1.TypedObjectReferenceÑ
Å∫kind∫name object˙g

apiGroup
	 string

kind
 stringä 

name
 stringä 

	namespace
	 string
¨
io.k8s.api.core.v1.Volumeé
ã∫name object˙˜
d
awsElasticBlockStoreLJ
H#/components/schemas/io.k8s.api.core.v1.AWSElasticBlockStoreVolumeSource
N
	azureDiskA?
=#/components/schemas/io.k8s.api.core.v1.AzureDiskVolumeSource
N
	azureFileA?
=#/components/schemas/io.k8s.api.core.v1.AzureFileVolumeSource
H
cephfs><
:#/components/schemas/io.k8s.api.core.v1.CephFSVolumeSource
H
cinder><
:#/components/schemas/io.k8s.api.core.v1.CinderVolumeSource
N
	configMapA?
=#/components/schemas/io.k8s.api.core.v1.ConfigMapVolumeSource
B
csi;9
7#/components/schemas/io.k8s.api.core.v1.CSIVolumeSource
R
downwardAPICA
?#/components/schemas/io.k8s.api.core.v1.DownwardAPIVolumeSource
L
emptyDir@>
<#/components/schemas/io.k8s.api.core.v1.EmptyDirVolumeSource
N
	ephemeralA?
=#/components/schemas/io.k8s.api.core.v1.EphemeralVolumeSource
@
fc:8
6#/components/schemas/io.k8s.api.core.v1.FCVolumeSource
J

flexVolume<:
8#/components/schemas/io.k8s.api.core.v1.FlexVolumeSource
J
flocker?=
;#/components/schemas/io.k8s.api.core.v1.FlockerVolumeSource
^
gcePersistentDiskIG
E#/components/schemas/io.k8s.api.core.v1.GCEPersistentDiskVolumeSource
J
gitRepo?=
;#/components/schemas/io.k8s.api.core.v1.GitRepoVolumeSource
N
	glusterfsA?
=#/components/schemas/io.k8s.api.core.v1.GlusterfsVolumeSource
L
hostPath@>
<#/components/schemas/io.k8s.api.core.v1.HostPathVolumeSource
F
image=;
9#/components/schemas/io.k8s.api.core.v1.ImageVolumeSource
F
iscsi=;
9#/components/schemas/io.k8s.api.core.v1.ISCSIVolumeSource

name
 stringä 
B
nfs;9
7#/components/schemas/io.k8s.api.core.v1.NFSVolumeSource
f
persistentVolumeClaimMK
I#/components/schemas/io.k8s.api.core.v1.PersistentVolumeClaimVolumeSource
d
photonPersistentDiskLJ
H#/components/schemas/io.k8s.api.core.v1.PhotonPersistentDiskVolumeSource
R
portworxVolume@>
<#/components/schemas/io.k8s.api.core.v1.PortworxVolumeSource
N
	projectedA?
=#/components/schemas/io.k8s.api.core.v1.ProjectedVolumeSource
J
quobyte?=
;#/components/schemas/io.k8s.api.core.v1.QuobyteVolumeSource
B
rbd;9
7#/components/schemas/io.k8s.api.core.v1.RBDVolumeSource
J
scaleIO?=
;#/components/schemas/io.k8s.api.core.v1.ScaleIOVolumeSource
H
secret><
:#/components/schemas/io.k8s.api.core.v1.SecretVolumeSource
N
	storageosA?
=#/components/schemas/io.k8s.api.core.v1.StorageOSVolumeSource
[
vsphereVolumeJH
F#/components/schemas/io.k8s.api.core.v1.VsphereVirtualDiskVolumeSource

io.k8s.api.core.v1.VolumeDevice\
Z∫name∫
devicePath object˙:


devicePath
 stringä 

name
 stringä 
¬
io.k8s.api.core.v1.VolumeMountü
ú∫name∫	mountPath object˙¸

	mountPath
 stringä 
Q
mountPropagation=
;¬Bidirectional
¬HostToContainer
¬None
 string

name
 stringä 

readOnly

 boolean
 
recursiveReadOnly
	 string

subPath
	 string

subPathExpr
	 string
∫
#io.k8s.api.core.v1.VolumeProjectioní
è object˙Ç
^
clusterTrustBundleHF
D#/components/schemas/io.k8s.api.core.v1.ClusterTrustBundleProjection
L
	configMap?=
;#/components/schemas/io.k8s.api.core.v1.ConfigMapProjection
P
downwardAPIA?
=#/components/schemas/io.k8s.api.core.v1.DownwardAPIProjection
V
podCertificateDB
@#/components/schemas/io.k8s.api.core.v1.PodCertificateProjection
F
secret<:
8#/components/schemas/io.k8s.api.core.v1.SecretProjection
`
serviceAccountTokenIG
E#/components/schemas/io.k8s.api.core.v1.ServiceAccountTokenProjection
à
-io.k8s.api.core.v1.VolumeResourceRequirements÷
” object˙∆
`
limitsV
T objectÇH
FD
B#/components/schemas/io.k8s.apimachinery.pkg.api.resource.Quantity
b
requestsV
T objectÇH
FD
B#/components/schemas/io.k8s.apimachinery.pkg.api.resource.Quantity
À
1io.k8s.api.core.v1.VsphereVirtualDiskVolumeSourceï
í∫
volumePath object˙y

fsType
	 string

storagePolicyID
	 string
 
storagePolicyName
	 string


volumePath
 stringä 
ﬁ
*io.k8s.api.core.v1.WeightedPodAffinityTermØ
¨∫weight∫podAffinityTerm object˙Ñ
V
podAffinityTermC
A“;9
7#/components/schemas/io.k8s.api.core.v1.PodAffinityTermä 
*
weight 
 integerä		        öint32
 
0io.k8s.api.core.v1.WindowsSecurityContextOptionsï
í object˙Ö
!
gmsaCredentialSpec
	 string
%
gmsaCredentialSpecName
	 string

hostProcess

 boolean

runAsUserName
	 string
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

‰
2io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptions≠
™ object˙ë
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
	 string¢à
x-kubernetes-group-version-kindec- group: ""
  kind: DeleteOptions
  version: v1
- group: batch
  kind: DeleteOptions
  version: v1

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
ƒ
/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEventê
ç∫type∫object object˙k
O
objectEC
A#/components/schemas/io.k8s.apimachinery.pkg.runtime.RawExtension

type
 stringä ¢Ç
x-kubernetes-group-version-kind_]- group: ""
  kind: WatchEvent
  version: v1
- group: batch
  kind: WatchEvent
  version: v1

;
,io.k8s.apimachinery.pkg.runtime.RawExtension
	 object
b
/io.k8s.apimachinery.pkg.util.intstr.IntOrString/
-⁄

 integer⁄
	 stringöint-or-string