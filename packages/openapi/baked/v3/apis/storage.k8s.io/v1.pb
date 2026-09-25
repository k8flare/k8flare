
3.0.0

Kubernetes2v1.36.4+k8flare"ßÏ
π
/apis/storage.k8s.io/v1/ú"ô

storage_v1get available resources*getStorageV1APIResourcesB◊‘
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
ã°
"/apis/storage.k8s.io/v1/csidrivers„†"Î>

storage_v1'list or watch objects of kind CSIDriver*listStorageV1CSIDriver2™
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
† booleanB˘ˆ
200Ó
Î
OK‰
R
application/json>
<:
8#/components/schemas/io.k8s.api.storage.v1.CSIDriverList
_
application/json;stream=watch>
<:
8#/components/schemas/io.k8s.api.storage.v1.CSIDriverList
e
#application/vnd.kubernetes.protobuf>
<:
8#/components/schemas/io.k8s.api.storage.v1.CSIDriverList
r
0application/vnd.kubernetes.protobuf;stream=watch>
<:
8#/components/schemas/io.k8s.api.storage.v1.CSIDriverList
R
application/yaml>
<:
8#/components/schemas/io.k8s.api.storage.v1.CSIDriverListj
x-kubernetes-actionlist
jW
x-kubernetes-group-version-kind42group: storage.k8s.io
version: v1
kind: CSIDriver
2Í

storage_v1create a CSIDriver*createStorageV1CSIDriver2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:I
GC
A
*/*:
86
4#/components/schemas/io.k8s.api.storage.v1.CSIDriverB”ï
200ç
ä
OKÉ
N
application/json:
86
4#/components/schemas/io.k8s.api.storage.v1.CSIDriver
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.storage.v1.CSIDriver
N
application/yaml:
86
4#/components/schemas/io.k8s.api.storage.v1.CSIDriverö
201í
è
CreatedÉ
N
application/json:
86
4#/components/schemas/io.k8s.api.storage.v1.CSIDriver
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.storage.v1.CSIDriver
N
application/yaml:
86
4#/components/schemas/io.k8s.api.storage.v1.CSIDriverõ
202ì
ê
AcceptedÉ
N
application/json:
86
4#/components/schemas/io.k8s.api.storage.v1.CSIDriver
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.storage.v1.CSIDriver
N
application/yaml:
86
4#/components/schemas/io.k8s.api.storage.v1.CSIDriverj
x-kubernetes-actionpost
jW
x-kubernetes-group-version-kind42group: storage.k8s.io
version: v1
kind: CSIDriver
:«K

storage_v1delete collection of CSIDriver*"deleteStorageV1CollectionCSIDriver2Ó	
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
jW
x-kubernetes-group-version-kind42group: storage.k8s.io
version: v1
kind: CSIDriver
jª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
ÁI
)/apis/storage.k8s.io/v1/csidrivers/{name}πI"’

storage_v1read the specified CSIDriver*readStorageV1CSIDriverBòï
200ç
ä
OKÉ
N
application/json:
86
4#/components/schemas/io.k8s.api.storage.v1.CSIDriver
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.storage.v1.CSIDriver
N
application/yaml:
86
4#/components/schemas/io.k8s.api.storage.v1.CSIDriverj
x-kubernetes-actionget
jW
x-kubernetes-group-version-kind42group: storage.k8s.io
version: v1
kind: CSIDriver
*Ÿ

storage_v1replace the specified CSIDriver*replaceStorageV1CSIDriver2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:I
GC
A
*/*:
86
4#/components/schemas/io.k8s.api.storage.v1.CSIDriverBµï
200ç
ä
OKÉ
N
application/json:
86
4#/components/schemas/io.k8s.api.storage.v1.CSIDriver
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.storage.v1.CSIDriver
N
application/yaml:
86
4#/components/schemas/io.k8s.api.storage.v1.CSIDriverö
201í
è
CreatedÉ
N
application/json:
86
4#/components/schemas/io.k8s.api.storage.v1.CSIDriver
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.storage.v1.CSIDriver
N
application/yaml:
86
4#/components/schemas/io.k8s.api.storage.v1.CSIDriverj
x-kubernetes-actionput
jW
x-kubernetes-group-version-kind42group: storage.k8s.io
version: v1
kind: CSIDriver
:ÿ

storage_v1delete a CSIDriver*deleteStorageV1CSIDriver2ù
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
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsB∂ï
200ç
ä
OKÉ
N
application/json:
86
4#/components/schemas/io.k8s.api.storage.v1.CSIDriver
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.storage.v1.CSIDriver
N
application/yaml:
86
4#/components/schemas/io.k8s.api.storage.v1.CSIDriverõ
202ì
ê
AcceptedÉ
N
application/json:
86
4#/components/schemas/io.k8s.api.storage.v1.CSIDriver
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.storage.v1.CSIDriver
N
application/yaml:
86
4#/components/schemas/io.k8s.api.storage.v1.CSIDriverj 
x-kubernetes-action	delete
jW
x-kubernetes-group-version-kind42group: storage.k8s.io
version: v1
kind: CSIDriver
R∞

storage_v1(partially update the specified CSIDriver*patchStorageV1CSIDriver2ù
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
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchBµï
200ç
ä
OKÉ
N
application/json:
86
4#/components/schemas/io.k8s.api.storage.v1.CSIDriver
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.storage.v1.CSIDriver
N
application/yaml:
86
4#/components/schemas/io.k8s.api.storage.v1.CSIDriverö
201í
è
CreatedÉ
N
application/json:
86
4#/components/schemas/io.k8s.api.storage.v1.CSIDriver
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.storage.v1.CSIDriver
N
application/yaml:
86
4#/components/schemas/io.k8s.api.storage.v1.CSIDriverj
x-kubernetes-actionpatch
jW
x-kubernetes-group-version-kind42group: storage.k8s.io
version: v1
kind: CSIDriver
j7
5
namepathname of the CSIDriver R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
Ÿ†
 /apis/storage.k8s.io/v1/csinodes≥†"€>

storage_v1%list or watch objects of kind CSINode*listStorageV1CSINode2™
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
† booleanBÔÏ
200‰
·
OK⁄
P
application/json<
:8
6#/components/schemas/io.k8s.api.storage.v1.CSINodeList
]
application/json;stream=watch<
:8
6#/components/schemas/io.k8s.api.storage.v1.CSINodeList
c
#application/vnd.kubernetes.protobuf<
:8
6#/components/schemas/io.k8s.api.storage.v1.CSINodeList
p
0application/vnd.kubernetes.protobuf;stream=watch<
:8
6#/components/schemas/io.k8s.api.storage.v1.CSINodeList
P
application/yaml<
:8
6#/components/schemas/io.k8s.api.storage.v1.CSINodeListj
x-kubernetes-actionlist
jU
x-kubernetes-group-version-kind20group: storage.k8s.io
version: v1
kind: CSINode
2–

storage_v1create a CSINode*createStorageV1CSINode2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:G
EA
?
*/*8
64
2#/components/schemas/io.k8s.api.storage.v1.CSINodeB¡è
200á
Ñ
OK˝
L
application/json8
64
2#/components/schemas/io.k8s.api.storage.v1.CSINode
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.storage.v1.CSINode
L
application/yaml8
64
2#/components/schemas/io.k8s.api.storage.v1.CSINodeî
201å
â
Created˝
L
application/json8
64
2#/components/schemas/io.k8s.api.storage.v1.CSINode
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.storage.v1.CSINode
L
application/yaml8
64
2#/components/schemas/io.k8s.api.storage.v1.CSINodeï
202ç
ä
Accepted˝
L
application/json8
64
2#/components/schemas/io.k8s.api.storage.v1.CSINode
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.storage.v1.CSINode
L
application/yaml8
64
2#/components/schemas/io.k8s.api.storage.v1.CSINodej
x-kubernetes-actionpost
jU
x-kubernetes-group-version-kind20group: storage.k8s.io
version: v1
kind: CSINode
:¡K

storage_v1delete collection of CSINode* deleteStorageV1CollectionCSINode2Ó	
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
jU
x-kubernetes-group-version-kind20group: storage.k8s.io
version: v1
kind: CSINode
jª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
üI
'/apis/storage.k8s.io/v1/csinodes/{name}ÛH"…

storage_v1read the specified CSINode*readStorageV1CSINodeBíè
200á
Ñ
OK˝
L
application/json8
64
2#/components/schemas/io.k8s.api.storage.v1.CSINode
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.storage.v1.CSINode
L
application/yaml8
64
2#/components/schemas/io.k8s.api.storage.v1.CSINodej
x-kubernetes-actionget
jU
x-kubernetes-group-version-kind20group: storage.k8s.io
version: v1
kind: CSINode
*≈

storage_v1replace the specified CSINode*replaceStorageV1CSINode2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:G
EA
?
*/*8
64
2#/components/schemas/io.k8s.api.storage.v1.CSINodeB©è
200á
Ñ
OK˝
L
application/json8
64
2#/components/schemas/io.k8s.api.storage.v1.CSINode
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.storage.v1.CSINode
L
application/yaml8
64
2#/components/schemas/io.k8s.api.storage.v1.CSINodeî
201å
â
Created˝
L
application/json8
64
2#/components/schemas/io.k8s.api.storage.v1.CSINode
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.storage.v1.CSINode
L
application/yaml8
64
2#/components/schemas/io.k8s.api.storage.v1.CSINodej
x-kubernetes-actionput
jU
x-kubernetes-group-version-kind20group: storage.k8s.io
version: v1
kind: CSINode
:∆

storage_v1delete a CSINode*deleteStorageV1CSINode2ù
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
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsB™è
200á
Ñ
OK˝
L
application/json8
64
2#/components/schemas/io.k8s.api.storage.v1.CSINode
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.storage.v1.CSINode
L
application/yaml8
64
2#/components/schemas/io.k8s.api.storage.v1.CSINodeï
202ç
ä
Accepted˝
L
application/json8
64
2#/components/schemas/io.k8s.api.storage.v1.CSINode
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.storage.v1.CSINode
L
application/yaml8
64
2#/components/schemas/io.k8s.api.storage.v1.CSINodej 
x-kubernetes-action	delete
jU
x-kubernetes-group-version-kind20group: storage.k8s.io
version: v1
kind: CSINode
Rû

storage_v1&partially update the specified CSINode*patchStorageV1CSINode2ù
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
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchB©è
200á
Ñ
OK˝
L
application/json8
64
2#/components/schemas/io.k8s.api.storage.v1.CSINode
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.storage.v1.CSINode
L
application/yaml8
64
2#/components/schemas/io.k8s.api.storage.v1.CSINodeî
201å
â
Created˝
L
application/json8
64
2#/components/schemas/io.k8s.api.storage.v1.CSINode
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.storage.v1.CSINode
L
application/yaml8
64
2#/components/schemas/io.k8s.api.storage.v1.CSINodej
x-kubernetes-actionpatch
jU
x-kubernetes-group-version-kind20group: storage.k8s.io
version: v1
kind: CSINode
j5
3
namepathname of the CSINode R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
µA
,/apis/storage.k8s.io/v1/csistoragecapacitiesÑA"ö

storage_v10list or watch objects of kind CSIStorageCapacity*/listStorageV1CSIStorageCapacityForAllNamespacesB¶£
200õ
ò
OKë
[
application/jsonG
EC
A#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacityList
h
application/json;stream=watchG
EC
A#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacityList
n
#application/vnd.kubernetes.protobufG
EC
A#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacityList
{
0application/vnd.kubernetes.protobuf;stream=watchG
EC
A#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacityList
[
application/yamlG
EC
A#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacityListj
x-kubernetes-actionlist
j`
x-kubernetes-group-version-kind=;group: storage.k8s.io
version: v1
kind: CSIStorageCapacity
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
Ö§
C/apis/storage.k8s.io/v1/namespaces/{namespace}/csistoragecapacitiesº£"Ω?

storage_v10list or watch objects of kind CSIStorageCapacity*)listStorageV1NamespacedCSIStorageCapacity2™
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
† booleanB¶£
200õ
ò
OKë
[
application/jsonG
EC
A#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacityList
h
application/json;stream=watchG
EC
A#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacityList
n
#application/vnd.kubernetes.protobufG
EC
A#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacityList
{
0application/vnd.kubernetes.protobuf;stream=watchG
EC
A#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacityList
[
application/yamlG
EC
A#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacityListj
x-kubernetes-actionlist
j`
x-kubernetes-group-version-kind=;group: storage.k8s.io
version: v1
kind: CSIStorageCapacity
2È

storage_v1create a CSIStorageCapacity*+createStorageV1NamespacedCSIStorageCapacity2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:R
PL
J
*/*C
A?
=#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacityB§∞
200®
•
OKû
W
application/jsonC
A?
=#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacity
j
#application/vnd.kubernetes.protobufC
A?
=#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacity
W
application/yamlC
A?
=#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacityµ
201≠
™
Createdû
W
application/jsonC
A?
=#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacity
j
#application/vnd.kubernetes.protobufC
A?
=#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacity
W
application/yamlC
A?
=#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacity∂
202Æ
´
Acceptedû
W
application/jsonC
A?
=#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacity
j
#application/vnd.kubernetes.protobufC
A?
=#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacity
W
application/yamlC
A?
=#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacityj
x-kubernetes-actionpost
j`
x-kubernetes-group-version-kind=;group: storage.k8s.io
version: v1
kind: CSIStorageCapacity
:ÏK

storage_v1'delete collection of CSIStorageCapacity*5deleteStorageV1CollectionNamespacedCSIStorageCapacity2Ó	
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
j`
x-kubernetes-group-version-kind=;group: storage.k8s.io
version: v1
kind: CSIStorageCapacity
ja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
ŒM
J/apis/storage.k8s.io/v1/namespaces/{namespace}/csistoragecapacities/{name}ˇL"ï

storage_v1%read the specified CSIStorageCapacity*)readStorageV1NamespacedCSIStorageCapacityB≥∞
200®
•
OKû
W
application/jsonC
A?
=#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacity
j
#application/vnd.kubernetes.protobufC
A?
=#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacity
W
application/yamlC
A?
=#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacityj
x-kubernetes-actionget
j`
x-kubernetes-group-version-kind=;group: storage.k8s.io
version: v1
kind: CSIStorageCapacity
*Ω

storage_v1(replace the specified CSIStorageCapacity*,replaceStorageV1NamespacedCSIStorageCapacity2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:R
PL
J
*/*C
A?
=#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacityBÎ∞
200®
•
OKû
W
application/jsonC
A?
=#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacity
j
#application/vnd.kubernetes.protobufC
A?
=#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacity
W
application/yamlC
A?
=#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacityµ
201≠
™
Createdû
W
application/jsonC
A?
=#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacity
j
#application/vnd.kubernetes.protobufC
A?
=#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacity
W
application/yamlC
A?
=#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacityj
x-kubernetes-actionput
j`
x-kubernetes-group-version-kind=;group: storage.k8s.io
version: v1
kind: CSIStorageCapacity
:≥

storage_v1delete a CSIStorageCapacity*+deleteStorageV1NamespacedCSIStorageCapacity2ù
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
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsBÏ∞
200®
•
OKû
W
application/jsonC
A?
=#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacity
j
#application/vnd.kubernetes.protobufC
A?
=#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacity
W
application/yamlC
A?
=#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacity∂
202Æ
´
Acceptedû
W
application/jsonC
A?
=#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacity
j
#application/vnd.kubernetes.protobufC
A?
=#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacity
W
application/yamlC
A?
=#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacityj 
x-kubernetes-action	delete
j`
x-kubernetes-group-version-kind=;group: storage.k8s.io
version: v1
kind: CSIStorageCapacity
Rã

storage_v11partially update the specified CSIStorageCapacity**patchStorageV1NamespacedCSIStorageCapacity2ù
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
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchBÎ∞
200®
•
OKû
W
application/jsonC
A?
=#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacity
j
#application/vnd.kubernetes.protobufC
A?
=#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacity
W
application/yamlC
A?
=#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacityµ
201≠
™
Createdû
W
application/jsonC
A?
=#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacity
j
#application/vnd.kubernetes.protobufC
A?
=#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacity
W
application/yamlC
A?
=#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacityj
x-kubernetes-actionpatch
j`
x-kubernetes-group-version-kind=;group: storage.k8s.io
version: v1
kind: CSIStorageCapacity
j@
>
namepathname of the CSIStorageCapacity R
† stringja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
◊°
&/apis/storage.k8s.io/v1/storageclasses´°"É?

storage_v1*list or watch objects of kind StorageClass*listStorageV1StorageClass2™
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
† booleanBàÖ
200˝
˙
OKÛ
U
application/jsonA
?=
;#/components/schemas/io.k8s.api.storage.v1.StorageClassList
b
application/json;stream=watchA
?=
;#/components/schemas/io.k8s.api.storage.v1.StorageClassList
h
#application/vnd.kubernetes.protobufA
?=
;#/components/schemas/io.k8s.api.storage.v1.StorageClassList
u
0application/vnd.kubernetes.protobuf;stream=watchA
?=
;#/components/schemas/io.k8s.api.storage.v1.StorageClassList
U
application/yamlA
?=
;#/components/schemas/io.k8s.api.storage.v1.StorageClassListj
x-kubernetes-actionlist
jZ
x-kubernetes-group-version-kind75group: storage.k8s.io
version: v1
kind: StorageClass
2ë

storage_v1create a StorageClass*createStorageV1StorageClass2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:L
JF
D
*/*=
;9
7#/components/schemas/io.k8s.api.storage.v1.StorageClassBÓû
200ñ
ì
OKå
Q
application/json=
;9
7#/components/schemas/io.k8s.api.storage.v1.StorageClass
d
#application/vnd.kubernetes.protobuf=
;9
7#/components/schemas/io.k8s.api.storage.v1.StorageClass
Q
application/yaml=
;9
7#/components/schemas/io.k8s.api.storage.v1.StorageClass£
201õ
ò
Createdå
Q
application/json=
;9
7#/components/schemas/io.k8s.api.storage.v1.StorageClass
d
#application/vnd.kubernetes.protobuf=
;9
7#/components/schemas/io.k8s.api.storage.v1.StorageClass
Q
application/yaml=
;9
7#/components/schemas/io.k8s.api.storage.v1.StorageClass§
202ú
ô
Acceptedå
Q
application/json=
;9
7#/components/schemas/io.k8s.api.storage.v1.StorageClass
d
#application/vnd.kubernetes.protobuf=
;9
7#/components/schemas/io.k8s.api.storage.v1.StorageClass
Q
application/yaml=
;9
7#/components/schemas/io.k8s.api.storage.v1.StorageClassj
x-kubernetes-actionpost
jZ
x-kubernetes-group-version-kind75group: storage.k8s.io
version: v1
kind: StorageClass
:–K

storage_v1!delete collection of StorageClass*%deleteStorageV1CollectionStorageClass2Ó	
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
jZ
x-kubernetes-group-version-kind75group: storage.k8s.io
version: v1
kind: StorageClass
jª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
‘J
-/apis/storage.k8s.io/v1/storageclasses/{name}¢J"Á

storage_v1read the specified StorageClass*readStorageV1StorageClassB°û
200ñ
ì
OKå
Q
application/json=
;9
7#/components/schemas/io.k8s.api.storage.v1.StorageClass
d
#application/vnd.kubernetes.protobuf=
;9
7#/components/schemas/io.k8s.api.storage.v1.StorageClass
Q
application/yaml=
;9
7#/components/schemas/io.k8s.api.storage.v1.StorageClassj
x-kubernetes-actionget
jZ
x-kubernetes-group-version-kind75group: storage.k8s.io
version: v1
kind: StorageClass
*˜

storage_v1"replace the specified StorageClass*replaceStorageV1StorageClass2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:L
JF
D
*/*=
;9
7#/components/schemas/io.k8s.api.storage.v1.StorageClassB«û
200ñ
ì
OKå
Q
application/json=
;9
7#/components/schemas/io.k8s.api.storage.v1.StorageClass
d
#application/vnd.kubernetes.protobuf=
;9
7#/components/schemas/io.k8s.api.storage.v1.StorageClass
Q
application/yaml=
;9
7#/components/schemas/io.k8s.api.storage.v1.StorageClass£
201õ
ò
Createdå
Q
application/json=
;9
7#/components/schemas/io.k8s.api.storage.v1.StorageClass
d
#application/vnd.kubernetes.protobuf=
;9
7#/components/schemas/io.k8s.api.storage.v1.StorageClass
Q
application/yaml=
;9
7#/components/schemas/io.k8s.api.storage.v1.StorageClassj
x-kubernetes-actionput
jZ
x-kubernetes-group-version-kind75group: storage.k8s.io
version: v1
kind: StorageClass
:Û

storage_v1delete a StorageClass*deleteStorageV1StorageClass2ù
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
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsB»û
200ñ
ì
OKå
Q
application/json=
;9
7#/components/schemas/io.k8s.api.storage.v1.StorageClass
d
#application/vnd.kubernetes.protobuf=
;9
7#/components/schemas/io.k8s.api.storage.v1.StorageClass
Q
application/yaml=
;9
7#/components/schemas/io.k8s.api.storage.v1.StorageClass§
202ú
ô
Acceptedå
Q
application/json=
;9
7#/components/schemas/io.k8s.api.storage.v1.StorageClass
d
#application/vnd.kubernetes.protobuf=
;9
7#/components/schemas/io.k8s.api.storage.v1.StorageClass
Q
application/yaml=
;9
7#/components/schemas/io.k8s.api.storage.v1.StorageClassj 
x-kubernetes-action	delete
jZ
x-kubernetes-group-version-kind75group: storage.k8s.io
version: v1
kind: StorageClass
RÀ

storage_v1+partially update the specified StorageClass*patchStorageV1StorageClass2ù
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
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchB«û
200ñ
ì
OKå
Q
application/json=
;9
7#/components/schemas/io.k8s.api.storage.v1.StorageClass
d
#application/vnd.kubernetes.protobuf=
;9
7#/components/schemas/io.k8s.api.storage.v1.StorageClass
Q
application/yaml=
;9
7#/components/schemas/io.k8s.api.storage.v1.StorageClass£
201õ
ò
Createdå
Q
application/json=
;9
7#/components/schemas/io.k8s.api.storage.v1.StorageClass
d
#application/vnd.kubernetes.protobuf=
;9
7#/components/schemas/io.k8s.api.storage.v1.StorageClass
Q
application/yaml=
;9
7#/components/schemas/io.k8s.api.storage.v1.StorageClassj
x-kubernetes-actionpatch
jZ
x-kubernetes-group-version-kind75group: storage.k8s.io
version: v1
kind: StorageClass
j:
8
namepathname of the StorageClass R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
∫¢
)/apis/storage.k8s.io/v1/volumeattachmentsã¢"£?

storage_v1.list or watch objects of kind VolumeAttachment*listStorageV1VolumeAttachment2™
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
† booleanBúô
200ë
é
OKá
Y
application/jsonE
CA
?#/components/schemas/io.k8s.api.storage.v1.VolumeAttachmentList
f
application/json;stream=watchE
CA
?#/components/schemas/io.k8s.api.storage.v1.VolumeAttachmentList
l
#application/vnd.kubernetes.protobufE
CA
?#/components/schemas/io.k8s.api.storage.v1.VolumeAttachmentList
y
0application/vnd.kubernetes.protobuf;stream=watchE
CA
?#/components/schemas/io.k8s.api.storage.v1.VolumeAttachmentList
Y
application/yamlE
CA
?#/components/schemas/io.k8s.api.storage.v1.VolumeAttachmentListj
x-kubernetes-actionlist
j^
x-kubernetes-group-version-kind;9group: storage.k8s.io
version: v1
kind: VolumeAttachment
2≈

storage_v1create a VolumeAttachment*createStorageV1VolumeAttachment2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:P
NJ
H
*/*A
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachmentBí™
200¢
ü
OKò
U
application/jsonA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachment
h
#application/vnd.kubernetes.protobufA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachment
U
application/yamlA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachmentØ
201ß
§
Createdò
U
application/jsonA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachment
h
#application/vnd.kubernetes.protobufA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachment
U
application/yamlA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachment∞
202®
•
Acceptedò
U
application/jsonA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachment
h
#application/vnd.kubernetes.protobufA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachment
U
application/yamlA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachmentj
x-kubernetes-actionpost
j^
x-kubernetes-group-version-kind;9group: storage.k8s.io
version: v1
kind: VolumeAttachment
:‹K

storage_v1%delete collection of VolumeAttachment*)deleteStorageV1CollectionVolumeAttachment2Ó	
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
j^
x-kubernetes-group-version-kind;9group: storage.k8s.io
version: v1
kind: VolumeAttachment
jª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
„K
0/apis/storage.k8s.io/v1/volumeattachments/{name}ÆK"ˇ

storage_v1#read the specified VolumeAttachment*readStorageV1VolumeAttachmentB≠™
200¢
ü
OKò
U
application/jsonA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachment
h
#application/vnd.kubernetes.protobufA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachment
U
application/yamlA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachmentj
x-kubernetes-actionget
j^
x-kubernetes-group-version-kind;9group: storage.k8s.io
version: v1
kind: VolumeAttachment
*ü

storage_v1&replace the specified VolumeAttachment* replaceStorageV1VolumeAttachment2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:P
NJ
H
*/*A
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachmentBﬂ™
200¢
ü
OKò
U
application/jsonA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachment
h
#application/vnd.kubernetes.protobufA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachment
U
application/yamlA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachmentØ
201ß
§
Createdò
U
application/jsonA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachment
h
#application/vnd.kubernetes.protobufA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachment
U
application/yamlA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachmentj
x-kubernetes-actionput
j^
x-kubernetes-group-version-kind;9group: storage.k8s.io
version: v1
kind: VolumeAttachment
:ó

storage_v1delete a VolumeAttachment*deleteStorageV1VolumeAttachment2ù
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
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsB‡™
200¢
ü
OKò
U
application/jsonA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachment
h
#application/vnd.kubernetes.protobufA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachment
U
application/yamlA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachment∞
202®
•
Acceptedò
U
application/jsonA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachment
h
#application/vnd.kubernetes.protobufA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachment
U
application/yamlA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachmentj 
x-kubernetes-action	delete
j^
x-kubernetes-group-version-kind;9group: storage.k8s.io
version: v1
kind: VolumeAttachment
RÔ

storage_v1/partially update the specified VolumeAttachment*patchStorageV1VolumeAttachment2ù
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
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchBﬂ™
200¢
ü
OKò
U
application/jsonA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachment
h
#application/vnd.kubernetes.protobufA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachment
U
application/yamlA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachmentØ
201ß
§
Createdò
U
application/jsonA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachment
h
#application/vnd.kubernetes.protobufA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachment
U
application/yamlA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachmentj
x-kubernetes-actionpatch
j^
x-kubernetes-group-version-kind;9group: storage.k8s.io
version: v1
kind: VolumeAttachment
j>
<
namepathname of the VolumeAttachment R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
Ä3
7/apis/storage.k8s.io/v1/volumeattachments/{name}/statusƒ2"è

storage_v1-read status of the specified VolumeAttachment*#readStorageV1VolumeAttachmentStatusB≠™
200¢
ü
OKò
U
application/jsonA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachment
h
#application/vnd.kubernetes.protobufA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachment
U
application/yamlA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachmentj
x-kubernetes-actionget
j^
x-kubernetes-group-version-kind;9group: storage.k8s.io
version: v1
kind: VolumeAttachment
*Ø

storage_v10replace status of the specified VolumeAttachment*&replaceStorageV1VolumeAttachmentStatus2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:P
NJ
H
*/*A
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachmentBﬂ™
200¢
ü
OKò
U
application/jsonA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachment
h
#application/vnd.kubernetes.protobufA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachment
U
application/yamlA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachmentØ
201ß
§
Createdò
U
application/jsonA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachment
h
#application/vnd.kubernetes.protobufA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachment
U
application/yamlA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachmentj
x-kubernetes-actionput
j^
x-kubernetes-group-version-kind;9group: storage.k8s.io
version: v1
kind: VolumeAttachment
Rˇ

storage_v19partially update status of the specified VolumeAttachment*$patchStorageV1VolumeAttachmentStatus2ù
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
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchBﬂ™
200¢
ü
OKò
U
application/jsonA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachment
h
#application/vnd.kubernetes.protobufA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachment
U
application/yamlA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachmentØ
201ß
§
Createdò
U
application/jsonA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachment
h
#application/vnd.kubernetes.protobufA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachment
U
application/yamlA
?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachmentj
x-kubernetes-actionpatch
j^
x-kubernetes-group-version-kind;9group: storage.k8s.io
version: v1
kind: VolumeAttachment
j>
<
namepathname of the VolumeAttachment R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
∏£
//apis/storage.k8s.io/v1/volumeattributesclassesÉ£"À?

storage_v13list or watch objects of kind VolumeAttributesClass*"listStorageV1VolumeAttributesClass2™
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
† booleanBµ≤
200™
ß
OK†
^
application/jsonJ
HF
D#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClassList
k
application/json;stream=watchJ
HF
D#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClassList
q
#application/vnd.kubernetes.protobufJ
HF
D#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClassList
~
0application/vnd.kubernetes.protobuf;stream=watchJ
HF
D#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClassList
^
application/yamlJ
HF
D#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClassListj
x-kubernetes-actionlist
jc
x-kubernetes-group-version-kind@>group: storage.k8s.io
version: v1
kind: VolumeAttributesClass
2Ü

storage_v1create a VolumeAttributesClass*$createStorageV1VolumeAttributesClass2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:U
SO
M
*/*F
DB
@#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClassBøπ
200±
Æ
OKß
Z
application/jsonF
DB
@#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClass
m
#application/vnd.kubernetes.protobufF
DB
@#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClass
Z
application/yamlF
DB
@#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClassæ
201∂
≥
Createdß
Z
application/jsonF
DB
@#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClass
m
#application/vnd.kubernetes.protobufF
DB
@#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClass
Z
application/yamlF
DB
@#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClassø
202∑
¥
Acceptedß
Z
application/jsonF
DB
@#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClass
m
#application/vnd.kubernetes.protobufF
DB
@#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClass
Z
application/yamlF
DB
@#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClassj
x-kubernetes-actionpost
jc
x-kubernetes-group-version-kind@>group: storage.k8s.io
version: v1
kind: VolumeAttributesClass
:ÎK

storage_v1*delete collection of VolumeAttributesClass*.deleteStorageV1CollectionVolumeAttributesClass2Ó	
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
jc
x-kubernetes-group-version-kind@>group: storage.k8s.io
version: v1
kind: VolumeAttributesClass
jª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
òM
6/apis/storage.k8s.io/v1/volumeattributesclasses/{name}›L"ù

storage_v1(read the specified VolumeAttributesClass*"readStorageV1VolumeAttributesClassBºπ
200±
Æ
OKß
Z
application/jsonF
DB
@#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClass
m
#application/vnd.kubernetes.protobufF
DB
@#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClass
Z
application/yamlF
DB
@#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClassj
x-kubernetes-actionget
jc
x-kubernetes-group-version-kind@>group: storage.k8s.io
version: v1
kind: VolumeAttributesClass
*—

storage_v1+replace the specified VolumeAttributesClass*%replaceStorageV1VolumeAttributesClass2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:U
SO
M
*/*F
DB
@#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClassB˝π
200±
Æ
OKß
Z
application/jsonF
DB
@#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClass
m
#application/vnd.kubernetes.protobufF
DB
@#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClass
Z
application/yamlF
DB
@#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClassæ
201∂
≥
Createdß
Z
application/jsonF
DB
@#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClass
m
#application/vnd.kubernetes.protobufF
DB
@#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClass
Z
application/yamlF
DB
@#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClassj
x-kubernetes-actionput
jc
x-kubernetes-group-version-kind@>group: storage.k8s.io
version: v1
kind: VolumeAttributesClass
:ƒ

storage_v1delete a VolumeAttributesClass*$deleteStorageV1VolumeAttributesClass2ù
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
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsB˛π
200±
Æ
OKß
Z
application/jsonF
DB
@#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClass
m
#application/vnd.kubernetes.protobufF
DB
@#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClass
Z
application/yamlF
DB
@#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClassø
202∑
¥
Acceptedß
Z
application/jsonF
DB
@#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClass
m
#application/vnd.kubernetes.protobufF
DB
@#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClass
Z
application/yamlF
DB
@#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClassj 
x-kubernetes-action	delete
jc
x-kubernetes-group-version-kind@>group: storage.k8s.io
version: v1
kind: VolumeAttributesClass
Rú

storage_v14partially update the specified VolumeAttributesClass*#patchStorageV1VolumeAttributesClass2ù
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
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchB˝π
200±
Æ
OKß
Z
application/jsonF
DB
@#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClass
m
#application/vnd.kubernetes.protobufF
DB
@#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClass
Z
application/yamlF
DB
@#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClassæ
201∂
≥
Createdß
Z
application/jsonF
DB
@#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClass
m
#application/vnd.kubernetes.protobufF
DB
@#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClass
Z
application/yamlF
DB
@#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClassj
x-kubernetes-actionpatch
jc
x-kubernetes-group-version-kind@>group: storage.k8s.io
version: v1
kind: VolumeAttributesClass
jC
A
namepath!name of the VolumeAttributesClass R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
ÌA
(/apis/storage.k8s.io/v1/watch/csidrivers¿A"÷

storage_v1uwatch individual changes to a list of CSIDriver. deprecated: use the 'watch' parameter with a list operation instead.*watchStorageV1CSIDriverListBµ≤
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
jW
x-kubernetes-group-version-kind42group: storage.k8s.io
version: v1
kind: CSIDriver
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
·B
//apis/storage.k8s.io/v1/watch/csidrivers/{name}≠B"ä

storage_v1∞watch changes to an object of kind CSIDriver. deprecated: use the 'watch' parameter with a list operation instead, filtered to a single item with the 'fieldSelector' parameter.*watchStorageV1CSIDriverBµ≤
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
jW
x-kubernetes-group-version-kind42group: storage.k8s.io
version: v1
kind: CSIDriver
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
† integerj7
5
namepathname of the CSIDriver R
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
ÂA
&/apis/storage.k8s.io/v1/watch/csinodes∫A"–

storage_v1swatch individual changes to a list of CSINode. deprecated: use the 'watch' parameter with a list operation instead.*watchStorageV1CSINodeListBµ≤
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
jU
x-kubernetes-group-version-kind20group: storage.k8s.io
version: v1
kind: CSINode
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
◊B
-/apis/storage.k8s.io/v1/watch/csinodes/{name}•B"Ñ

storage_v1Æwatch changes to an object of kind CSINode. deprecated: use the 'watch' parameter with a list operation instead, filtered to a single item with the 'fieldSelector' parameter.*watchStorageV1CSINodeBµ≤
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
jU
x-kubernetes-group-version-kind20group: storage.k8s.io
version: v1
kind: CSINode
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
namepathname of the CSINode R
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
¢B
2/apis/storage.k8s.io/v1/watch/csistoragecapacitiesÎA"Å

storage_v1~watch individual changes to a list of CSIStorageCapacity. deprecated: use the 'watch' parameter with a list operation instead.*4watchStorageV1CSIStorageCapacityListForAllNamespacesBµ≤
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
j`
x-kubernetes-group-version-kind=;group: storage.k8s.io
version: v1
kind: CSIStorageCapacity
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
ñC
I/apis/storage.k8s.io/v1/watch/namespaces/{namespace}/csistoragecapacities»B"˚

storage_v1~watch individual changes to a list of CSIStorageCapacity. deprecated: use the 'watch' parameter with a list operation instead.*.watchStorageV1NamespacedCSIStorageCapacityListBµ≤
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
j`
x-kubernetes-group-version-kind=;group: storage.k8s.io
version: v1
kind: CSIStorageCapacity
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
ìD
P/apis/storage.k8s.io/v1/watch/namespaces/{namespace}/csistoragecapacities/{name}æC"Ø

storage_v1πwatch changes to an object of kind CSIStorageCapacity. deprecated: use the 'watch' parameter with a list operation instead, filtered to a single item with the 'fieldSelector' parameter.**watchStorageV1NamespacedCSIStorageCapacityBµ≤
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
j`
x-kubernetes-group-version-kind=;group: storage.k8s.io
version: v1
kind: CSIStorageCapacity
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
† integerj@
>
namepathname of the CSIStorageCapacity R
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
˙A
,/apis/storage.k8s.io/v1/watch/storageclasses…A"ﬂ

storage_v1xwatch individual changes to a list of StorageClass. deprecated: use the 'watch' parameter with a list operation instead.*watchStorageV1StorageClassListBµ≤
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
jZ
x-kubernetes-group-version-kind75group: storage.k8s.io
version: v1
kind: StorageClass
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
ÒB
3/apis/storage.k8s.io/v1/watch/storageclasses/{name}πB"ì

storage_v1≥watch changes to an object of kind StorageClass. deprecated: use the 'watch' parameter with a list operation instead, filtered to a single item with the 'fieldSelector' parameter.*watchStorageV1StorageClassBµ≤
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
jZ
x-kubernetes-group-version-kind75group: storage.k8s.io
version: v1
kind: StorageClass
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
† integerj:
8
namepathname of the StorageClass R
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
âB
//apis/storage.k8s.io/v1/watch/volumeattachments’A"Î

storage_v1|watch individual changes to a list of VolumeAttachment. deprecated: use the 'watch' parameter with a list operation instead.*"watchStorageV1VolumeAttachmentListBµ≤
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
j^
x-kubernetes-group-version-kind;9group: storage.k8s.io
version: v1
kind: VolumeAttachment
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
ÑC
6/apis/storage.k8s.io/v1/watch/volumeattachments/{name}…B"ü

storage_v1∑watch changes to an object of kind VolumeAttachment. deprecated: use the 'watch' parameter with a list operation instead, filtered to a single item with the 'fieldSelector' parameter.*watchStorageV1VolumeAttachmentBµ≤
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
j^
x-kubernetes-group-version-kind;9group: storage.k8s.io
version: v1
kind: VolumeAttachment
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
† integerj>
<
namepathname of the VolumeAttachment R
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
üB
5/apis/storage.k8s.io/v1/watch/volumeattributesclassesÂA"˚

storage_v1Åwatch individual changes to a list of VolumeAttributesClass. deprecated: use the 'watch' parameter with a list operation instead.*'watchStorageV1VolumeAttributesClassListBµ≤
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
jc
x-kubernetes-group-version-kind@>group: storage.k8s.io
version: v1
kind: VolumeAttributesClass
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
ûC
</apis/storage.k8s.io/v1/watch/volumeattributesclasses/{name}›B"Æ

storage_v1ºwatch changes to an object of kind VolumeAttributesClass. deprecated: use the 'watch' parameter with a list operation instead, filtered to a single item with the 'fieldSelector' parameter.*#watchStorageV1VolumeAttributesClassBµ≤
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
jc
x-kubernetes-group-version-kind@>group: storage.k8s.io
version: v1
kind: VolumeAttributesClass
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
† integerjC
A
namepath!name of the VolumeAttributesClass R
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
† boolean*¿¡
º¡
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
ÿ
2io.k8s.api.core.v1.AzureFilePersistentVolumeSource°
û∫
secretName∫	shareName object˙y

readOnly

 boolean


secretName
 stringä 

secretNamespace
	 string

	shareName
 stringä 
Ø
,io.k8s.api.core.v1.CSIPersistentVolumeSource˛
˚∫driver∫volumeHandle object˙÷
X
controllerExpandSecretRef;9
7#/components/schemas/io.k8s.api.core.v1.SecretReference
Y
controllerPublishSecretRef;9
7#/components/schemas/io.k8s.api.core.v1.SecretReference

driver
 stringä 

fsType
	 string
R
nodeExpandSecretRef;9
7#/components/schemas/io.k8s.api.core.v1.SecretReference
S
nodePublishSecretRef;9
7#/components/schemas/io.k8s.api.core.v1.SecretReference
Q
nodeStageSecretRef;9
7#/components/schemas/io.k8s.api.core.v1.SecretReference

readOnly

 boolean
4
volumeAttributes 
 objectÇ

 stringä 
 
volumeHandle
 stringä 
À
/io.k8s.api.core.v1.CephFSPersistentVolumeSourceó
î∫monitors object˙¸
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
H
	secretRef;9
7#/components/schemas/io.k8s.api.core.v1.SecretReference

user
	 string
Ë
/io.k8s.api.core.v1.CinderPersistentVolumeSource¥
±∫volumeID object˙ô

fsType
	 string

readOnly

 boolean
H
	secretRef;9
7#/components/schemas/io.k8s.api.core.v1.SecretReference

volumeID
 stringä 
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

è
-io.k8s.api.core.v1.FlexPersistentVolumeSource›
⁄∫driver object˙ƒ
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
H
	secretRef;9
7#/components/schemas/io.k8s.api.core.v1.SecretReference
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
œ
2io.k8s.api.core.v1.GlusterfsPersistentVolumeSourceò
ï∫	endpoints∫path object˙v

	endpoints
 stringä 
!
endpointsNamespace
	 string

path
 stringä 

readOnly

 boolean
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
ú
.io.k8s.api.core.v1.ISCSIPersistentVolumeSourceÈ
Ê∫targetPortal∫iqn∫lun object˙æ
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
H
	secretRef;9
7#/components/schemas/io.k8s.api.core.v1.SecretReference
 
targetPortal
 stringä 
n
$io.k8s.api.core.v1.LocalVolumeSourceF
D∫path object˙1

fsType
	 string

path
 stringä 
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

Ë
'io.k8s.api.core.v1.PersistentVolumeSpecº
π object˙¨
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

d
awsElasticBlockStoreLJ
H#/components/schemas/io.k8s.api.core.v1.AWSElasticBlockStoreVolumeSource
N
	azureDiskA?
=#/components/schemas/io.k8s.api.core.v1.AzureDiskVolumeSource
X
	azureFileKI
G#/components/schemas/io.k8s.api.core.v1.AzureFilePersistentVolumeSource
b
capacityV
T objectÇH
FD
B#/components/schemas/io.k8s.apimachinery.pkg.api.resource.Quantity
R
cephfsHF
D#/components/schemas/io.k8s.api.core.v1.CephFSPersistentVolumeSource
R
cinderHF
D#/components/schemas/io.k8s.api.core.v1.CinderPersistentVolumeSource
s
claimRefg
e“;9
7#/components/schemas/io.k8s.api.core.v1.ObjectReference¢$
x-kubernetes-map-type	granular

L
csiEC
A#/components/schemas/io.k8s.api.core.v1.CSIPersistentVolumeSource
@
fc:8
6#/components/schemas/io.k8s.api.core.v1.FCVolumeSource
T

flexVolumeFD
B#/components/schemas/io.k8s.api.core.v1.FlexPersistentVolumeSource
J
flocker?=
;#/components/schemas/io.k8s.api.core.v1.FlockerVolumeSource
^
gcePersistentDiskIG
E#/components/schemas/io.k8s.api.core.v1.GCEPersistentDiskVolumeSource
X
	glusterfsKI
G#/components/schemas/io.k8s.api.core.v1.GlusterfsPersistentVolumeSource
L
hostPath@>
<#/components/schemas/io.k8s.api.core.v1.HostPathVolumeSource
P
iscsiGE
C#/components/schemas/io.k8s.api.core.v1.ISCSIPersistentVolumeSource
F
local=;
9#/components/schemas/io.k8s.api.core.v1.LocalVolumeSource
U
mountOptionsE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

B
nfs;9
7#/components/schemas/io.k8s.api.core.v1.NFSVolumeSource
N
nodeAffinity><
:#/components/schemas/io.k8s.api.core.v1.VolumeNodeAffinity
Q
persistentVolumeReclaimPolicy0
.¬	Delete
¬
Recycle
¬	Retain
 string
d
photonPersistentDiskLJ
H#/components/schemas/io.k8s.api.core.v1.PhotonPersistentDiskVolumeSource
R
portworxVolume@>
<#/components/schemas/io.k8s.api.core.v1.PortworxVolumeSource
J
quobyte?=
;#/components/schemas/io.k8s.api.core.v1.QuobyteVolumeSource
L
rbdEC
A#/components/schemas/io.k8s.api.core.v1.RBDPersistentVolumeSource
T
scaleIOIG
E#/components/schemas/io.k8s.api.core.v1.ScaleIOPersistentVolumeSource

storageClassName
	 string
X
	storageosKI
G#/components/schemas/io.k8s.api.core.v1.StorageOSPersistentVolumeSource
(
volumeAttributesClassName
	 string
4

volumeMode&
$¬Block
¬Filesystem
 string
[
vsphereVolumeJH
F#/components/schemas/io.k8s.api.core.v1.VsphereVirtualDiskVolumeSource
}
3io.k8s.api.core.v1.PhotonPersistentDiskVolumeSourceF
D∫pdID object˙1

fsType
	 string

pdID
 stringä 
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
ß
,io.k8s.api.core.v1.RBDPersistentVolumeSourceˆ
Û∫monitors∫image object˙”
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
H
	secretRef;9
7#/components/schemas/io.k8s.api.core.v1.SecretReference

user
 stringäadmin
ƒ
0io.k8s.api.core.v1.ScaleIOPersistentVolumeSourceè
å∫gateway∫system∫	secretRef object˙‡

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
H
	secretRef;9
7#/components/schemas/io.k8s.api.core.v1.SecretReference
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
à
"io.k8s.api.core.v1.SecretReferenceb
` object˙/

name
	 string

	namespace
	 string¢"
x-kubernetes-map-type	atomic

˝
2io.k8s.api.core.v1.StorageOSPersistentVolumeSource∆
√ object˙∂

fsType
	 string

readOnly

 boolean
H
	secretRef;9
7#/components/schemas/io.k8s.api.core.v1.ObjectReference


volumeName
	 string

volumeNamespace
	 string
¿
3io.k8s.api.core.v1.TopologySelectorLabelRequirementà
Ö∫key∫values object˙j

key
 stringä 
O
valuesE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

à
'io.k8s.api.core.v1.TopologySelectorTerm‹
Ÿ object˙ß
§
matchLabelExpressionsä
á arrayÚV
T
R“LJ
H#/components/schemas/io.k8s.api.core.v1.TopologySelectorLabelRequirementä ¢#
x-kubernetes-list-type	atomic
¢"
x-kubernetes-map-type	atomic

}
%io.k8s.api.core.v1.VolumeNodeAffinityT
R object˙F
D
required86
4#/components/schemas/io.k8s.api.core.v1.NodeSelector
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
˜
io.k8s.api.storage.v1.CSIDriver”
–∫spec object˙‹
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
L
specD
B“<:
8#/components/schemas/io.k8s.api.storage.v1.CSIDriverSpecä ¢]
x-kubernetes-group-version-kind:8- group: storage.k8s.io
  kind: CSIDriver
  version: v1

ä
#io.k8s.api.storage.v1.CSIDriverList‚
ﬂ∫items object˙Ê


apiVersion
	 string
X
itemsO
M arrayÚB
@
>“86
4#/components/schemas/io.k8s.api.storage.v1.CSIDriverä 

kind
	 string
Z
metadataN
L“FD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaä ¢a
x-kubernetes-group-version-kind><- group: storage.k8s.io
  kind: CSIDriverList
  version: v1

˘
#io.k8s.api.storage.v1.CSIDriverSpec—
Œ object˙¡

attachRequired

 boolean

fsGroupPolicy
	 string
:
"nodeAllocatableUpdatePeriodSeconds
 integeröint64

podInfoOnMount

 boolean
-
preventPodSchedulingIfMissing

 boolean
!
requiresRepublish

 boolean

seLinuxMount

 boolean
,
serviceAccountTokenInSecrets

 boolean

storageCapacity

 boolean
â
tokenRequestsx
v arrayÚE
C
A“;9
7#/components/schemas/io.k8s.api.storage.v1.TokenRequestä ¢#
x-kubernetes-list-type	atomic

Z
volumeLifecycleModesB
@ arrayÚ

 stringä ¢ 
x-kubernetes-list-typeset

Ò
io.k8s.api.storage.v1.CSINodeœ
Ã∫spec object˙⁄
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
J
specB
@“:8
6#/components/schemas/io.k8s.api.storage.v1.CSINodeSpecä ¢[
x-kubernetes-group-version-kind86- group: storage.k8s.io
  kind: CSINode
  version: v1

®
#io.k8s.api.storage.v1.CSINodeDriverÄ
˝∫name∫nodeID object˙‡
Q
allocatableB@
>#/components/schemas/io.k8s.api.storage.v1.VolumeNodeResources

name
 stringä 

nodeID
 stringä 
U
topologyKeysE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

Ñ
!io.k8s.api.storage.v1.CSINodeListﬁ
€∫items object˙‰


apiVersion
	 string
V
itemsM
K arrayÚ@
>
<“64
2#/components/schemas/io.k8s.api.storage.v1.CSINodeä 

kind
	 string
Z
metadataN
L“FD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaä ¢_
x-kubernetes-group-version-kind<:- group: storage.k8s.io
  kind: CSINodeList
  version: v1

ƒ
!io.k8s.api.storage.v1.CSINodeSpecû
õ∫drivers object˙Ñ
Å
driversı
Ú arrayÚF
D
B“<:
8#/components/schemas/io.k8s.api.storage.v1.CSINodeDriverä ¢'
x-kubernetes-list-map-keys	- name
¢ 
x-kubernetes-list-typemap
¢'
x-kubernetes-patch-merge-keyname
¢'
x-kubernetes-patch-strategymerge

˚
(io.k8s.api.storage.v1.CSIStorageCapacityŒ
À∫storageClassName object˙¬


apiVersion
	 string
R
capacityFD
B#/components/schemas/io.k8s.apimachinery.pkg.api.resource.Quantity

kind
	 string
[
maximumVolumeSizeFD
B#/components/schemas/io.k8s.apimachinery.pkg.api.resource.Quantity
\
metadataP
N“HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMetaä 
[
nodeTopologyKI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.LabelSelector
$
storageClassName
 stringä ¢f
x-kubernetes-group-version-kindCA- group: storage.k8s.io
  kind: CSIStorageCapacity
  version: v1

•
,io.k8s.api.storage.v1.CSIStorageCapacityListÙ
Ò∫items object˙Ô


apiVersion
	 string
a
itemsX
V arrayÚK
I
G“A?
=#/components/schemas/io.k8s.api.storage.v1.CSIStorageCapacityä 

kind
	 string
Z
metadataN
L“FD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaä ¢j
x-kubernetes-group-version-kindGE- group: storage.k8s.io
  kind: CSIStorageCapacityList
  version: v1

ß
"io.k8s.api.storage.v1.StorageClassÄ
˝∫provisioner object˙ˇ
$
allowVolumeExpansion

 boolean
í
allowedTopologies}
{ arrayÚJ
H
F“@>
<#/components/schemas/io.k8s.api.core.v1.TopologySelectorTermä ¢#
x-kubernetes-list-type	atomic

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
U
mountOptionsE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

.

parameters 
 objectÇ

 stringä 

provisioner
 stringä 
A
reclaimPolicy0
.¬	Delete
¬
Recycle
¬	Retain
 string
I
volumeBindingMode4
2¬
Immediate
¬WaitForFirstConsumer
 string¢`
x-kubernetes-group-version-kind=;- group: storage.k8s.io
  kind: StorageClass
  version: v1

ì
&io.k8s.api.storage.v1.StorageClassListË
Â∫items object˙È


apiVersion
	 string
[
itemsR
P arrayÚE
C
A“;9
7#/components/schemas/io.k8s.api.storage.v1.StorageClassä 

kind
	 string
Z
metadataN
L“FD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaä ¢d
x-kubernetes-group-version-kindA?- group: storage.k8s.io
  kind: StorageClassList
  version: v1

à
"io.k8s.api.storage.v1.TokenRequestb
`∫audience object˙I

audience
 stringä 
)
expirationSeconds
 integeröint64
Â
&io.k8s.api.storage.v1.VolumeAttachment∫
∑∫spec object˙º
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
S
specK
I“CA
?#/components/schemas/io.k8s.api.storage.v1.VolumeAttachmentSpecä 
W
statusM
K“EC
A#/components/schemas/io.k8s.api.storage.v1.VolumeAttachmentStatusä ¢d
x-kubernetes-group-version-kindA?- group: storage.k8s.io
  kind: VolumeAttachment
  version: v1

ü
*io.k8s.api.storage.v1.VolumeAttachmentList
Ì∫items object˙Ì


apiVersion
	 string
_
itemsV
T arrayÚI
G
E“?=
;#/components/schemas/io.k8s.api.storage.v1.VolumeAttachmentä 

kind
	 string
Z
metadataN
L“FD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaä ¢h
x-kubernetes-group-version-kindEC- group: storage.k8s.io
  kind: VolumeAttachmentList
  version: v1

ª
,io.k8s.api.storage.v1.VolumeAttachmentSourceä
á object˙{
T
inlineVolumeSpec@>
<#/components/schemas/io.k8s.api.core.v1.PersistentVolumeSpec
#
persistentVolumeName
	 string
Û
*io.k8s.api.storage.v1.VolumeAttachmentSpecƒ
¡∫attacher∫source∫nodeName object˙ï

attacher
 stringä 

nodeName
 stringä 
W
sourceM
K“EC
A#/components/schemas/io.k8s.api.storage.v1.VolumeAttachmentSourceä 
π
,io.k8s.api.storage.v1.VolumeAttachmentStatusà
Ö∫attached object˙Ì
I
attachError:8
6#/components/schemas/io.k8s.api.storage.v1.VolumeError

attached
 booleanä 
6
attachmentMetadata 
 objectÇ

 stringä 
I
detachError:8
6#/components/schemas/io.k8s.api.storage.v1.VolumeError
ó
+io.k8s.api.storage.v1.VolumeAttributesClassÁ
‰∫
driverName object˙ﬁ


apiVersion
	 string


driverName
 stringä 

kind
	 string
\
metadataP
N“HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMetaä 
.

parameters 
 objectÇ

 stringä ¢i
x-kubernetes-group-version-kindFD- group: storage.k8s.io
  kind: VolumeAttributesClass
  version: v1

Æ
/io.k8s.api.storage.v1.VolumeAttributesClassList˙
˜∫items object˙Ú


apiVersion
	 string
d
items[
Y arrayÚN
L
J“DB
@#/components/schemas/io.k8s.api.storage.v1.VolumeAttributesClassä 

kind
	 string
Z
metadataN
L“FD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaä ¢m
x-kubernetes-group-version-kindJH- group: storage.k8s.io
  kind: VolumeAttributesClassList
  version: v1

Ω
!io.k8s.api.storage.v1.VolumeErroró
î object˙á
!
	errorCode
 integeröint32

message
	 string
J
timeB@
>#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Time
Z
)io.k8s.api.storage.v1.VolumeNodeResources-
+ object˙

count
 integeröint32
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

Ì
2io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptions∂
≥ object˙ë
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
	 string¢ë
x-kubernetes-group-version-kindnl- group: ""
  kind: DeleteOptions
  version: v1
- group: storage.k8s.io
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
Õ
/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEventô
ñ∫type∫object object˙k
O
objectEC
A#/components/schemas/io.k8s.apimachinery.pkg.runtime.RawExtension

type
 stringä ¢ã
x-kubernetes-group-version-kindhf- group: ""
  kind: WatchEvent
  version: v1
- group: storage.k8s.io
  kind: WatchEvent
  version: v1

;
,io.k8s.apimachinery.pkg.runtime.RawExtension
	 object