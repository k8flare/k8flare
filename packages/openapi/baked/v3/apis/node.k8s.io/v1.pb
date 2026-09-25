
3.0.0

Kubernetes2v1.36.4+k8flare"Šó
°
/apis/node.k8s.io/v1/–"“
node_v1get available resources*getNodeV1APIResourcesB×Ô
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
Œ¡
#/apis/node.k8s.io/v1/runtimeclassesã "ë>
node_v1*list or watch objects of kind RuntimeClass*listNodeV1RuntimeClass2ª
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
 ÊbooleanBùö
200î
ë
OKä
R
application/json>
<:
8#/components/schemas/io.k8s.api.node.v1.RuntimeClassList
_
application/json;stream=watch>
<:
8#/components/schemas/io.k8s.api.node.v1.RuntimeClassList
e
#application/vnd.kubernetes.protobuf>
<:
8#/components/schemas/io.k8s.api.node.v1.RuntimeClassList
r
0application/vnd.kubernetes.protobuf;stream=watch>
<:
8#/components/schemas/io.k8s.api.node.v1.RuntimeClassList
R
application/yaml>
<:
8#/components/schemas/io.k8s.api.node.v1.RuntimeClassListj
x-kubernetes-actionlist
jW
x-kubernetes-group-version-kind42group: node.k8s.io
version: v1
kind: RuntimeClass
2ê
node_v1create a RuntimeClass*createNodeV1RuntimeClass2
š
dryRunqueryøWhen present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
 Êstring2•
’
fieldManagerqueryêfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
 Êstring2Û
Ø
fieldValidationquery­fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
 Êstring:I
GC
A
*/*:
86
4#/components/schemas/io.k8s.api.node.v1.RuntimeClassBÓ•
200
Š
OKƒ
N
application/json:
86
4#/components/schemas/io.k8s.api.node.v1.RuntimeClass
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.node.v1.RuntimeClass
N
application/yaml:
86
4#/components/schemas/io.k8s.api.node.v1.RuntimeClassš
201’

Createdƒ
N
application/json:
86
4#/components/schemas/io.k8s.api.node.v1.RuntimeClass
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.node.v1.RuntimeClass
N
application/yaml:
86
4#/components/schemas/io.k8s.api.node.v1.RuntimeClass›
202“

Acceptedƒ
N
application/json:
86
4#/components/schemas/io.k8s.api.node.v1.RuntimeClass
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.node.v1.RuntimeClass
N
application/yaml:
86
4#/components/schemas/io.k8s.api.node.v1.RuntimeClassj
x-kubernetes-actionpost
jW
x-kubernetes-group-version-kind42group: node.k8s.io
version: v1
kind: RuntimeClass
:ÇK
node_v1!delete collection of RuntimeClass*"deleteNodeV1CollectionRuntimeClass2î	
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
jW
x-kubernetes-group-version-kind42group: node.k8s.io
version: v1
kind: RuntimeClass
j»
¸
prettyquery–If 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
 Êstring
ëI
*/apis/node.k8s.io/v1/runtimeclasses/{name}¼I"Õ
node_v1read the specified RuntimeClass*readNodeV1RuntimeClassB˜•
200
Š
OKƒ
N
application/json:
86
4#/components/schemas/io.k8s.api.node.v1.RuntimeClass
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.node.v1.RuntimeClass
N
application/yaml:
86
4#/components/schemas/io.k8s.api.node.v1.RuntimeClassj
x-kubernetes-actionget
jW
x-kubernetes-group-version-kind42group: node.k8s.io
version: v1
kind: RuntimeClass
*Ù
node_v1"replace the specified RuntimeClass*replaceNodeV1RuntimeClass2
š
dryRunqueryøWhen present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
 Êstring2•
’
fieldManagerqueryêfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
 Êstring2Û
Ø
fieldValidationquery­fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
 Êstring:I
GC
A
*/*:
86
4#/components/schemas/io.k8s.api.node.v1.RuntimeClassBµ•
200
Š
OKƒ
N
application/json:
86
4#/components/schemas/io.k8s.api.node.v1.RuntimeClass
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.node.v1.RuntimeClass
N
application/yaml:
86
4#/components/schemas/io.k8s.api.node.v1.RuntimeClassš
201’

Createdƒ
N
application/json:
86
4#/components/schemas/io.k8s.api.node.v1.RuntimeClass
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.node.v1.RuntimeClass
N
application/yaml:
86
4#/components/schemas/io.k8s.api.node.v1.RuntimeClassj
x-kubernetes-actionput
jW
x-kubernetes-group-version-kind42group: node.k8s.io
version: v1
kind: RuntimeClass
:Ø
node_v1delete a RuntimeClass*deleteNodeV1RuntimeClass2
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
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsB¶•
200
Š
OKƒ
N
application/json:
86
4#/components/schemas/io.k8s.api.node.v1.RuntimeClass
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.node.v1.RuntimeClass
N
application/yaml:
86
4#/components/schemas/io.k8s.api.node.v1.RuntimeClass›
202“

Acceptedƒ
N
application/json:
86
4#/components/schemas/io.k8s.api.node.v1.RuntimeClass
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.node.v1.RuntimeClass
N
application/yaml:
86
4#/components/schemas/io.k8s.api.node.v1.RuntimeClassj 
x-kubernetes-action	delete
jW
x-kubernetes-group-version-kind42group: node.k8s.io
version: v1
kind: RuntimeClass
R°
node_v1+partially update the specified RuntimeClass*patchNodeV1RuntimeClass2
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
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchBµ•
200
Š
OKƒ
N
application/json:
86
4#/components/schemas/io.k8s.api.node.v1.RuntimeClass
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.node.v1.RuntimeClass
N
application/yaml:
86
4#/components/schemas/io.k8s.api.node.v1.RuntimeClassš
201’

Createdƒ
N
application/json:
86
4#/components/schemas/io.k8s.api.node.v1.RuntimeClass
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.node.v1.RuntimeClass
N
application/yaml:
86
4#/components/schemas/io.k8s.api.node.v1.RuntimeClassj
x-kubernetes-actionpatch
jW
x-kubernetes-group-version-kind42group: node.k8s.io
version: v1
kind: RuntimeClass
j:
8
namepathname of the RuntimeClass R
 Êstringj»
¸
prettyquery–If 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
 Êstring
îA
)/apis/node.k8s.io/v1/watch/runtimeclassesÀA"Ö
node_v1xwatch individual changes to a list of RuntimeClass. deprecated: use the 'watch' parameter with a list operation instead.*watchNodeV1RuntimeClassListBµ²
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
jW
x-kubernetes-group-version-kind42group: node.k8s.io
version: v1
kind: RuntimeClass
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
åB
0/apis/node.k8s.io/v1/watch/runtimeclasses/{name}°B"Š
node_v1³watch changes to an object of kind RuntimeClass. deprecated: use the 'watch' parameter with a list operation instead, filtered to a single item with the 'fieldSelector' parameter.*watchNodeV1RuntimeClassBµ²
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
jW
x-kubernetes-group-version-kind42group: node.k8s.io
version: v1
kind: RuntimeClass
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
 Êintegerj:
8
namepathname of the RuntimeClass R
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
 Êboolean*Ú7
×7
“
io.k8s.api.core.v1.Tolerationñ
îÊobjectúá
J
effect@
>Â
NoExecute
ÂNoSchedule
ÂPreferNoSchedule
Êstring

key
	Êstring
>
operator2
0ÂEqual
Â	Exists
ÂGt
ÂLt
Êstring
)
tolerationSeconds
Êintegeršint64

value
	Êstring
‘
io.k8s.api.node.v1.Overheadr
pÊobjectúd
b
podFixedV
TÊobject‚H
FD
B#/components/schemas/io.k8s.apimachinery.pkg.api.resource.Quantity
Ñ
io.k8s.api.node.v1.RuntimeClass­
ªºhandlerÊobjectú³


apiVersion
	Êstring

handler
ÊstringŠ 

kind
	Êstring
\
metadataP
NÒHF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMetaŠ 
@
overhead42
0#/components/schemas/io.k8s.api.node.v1.Overhead
D

scheduling64
2#/components/schemas/io.k8s.api.node.v1.Scheduling¢]
x-kubernetes-group-version-kind:8- group: node.k8s.io
  kind: RuntimeClass
  version: v1

Š
#io.k8s.api.node.v1.RuntimeClassListâ
ßºitemsÊobjectúæ


apiVersion
	Êstring
X
itemsO
MÊarrayòB
@
>Ò86
4#/components/schemas/io.k8s.api.node.v1.RuntimeClassŠ 

kind
	Êstring
Z
metadataN
LÒFD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaŠ ¢a
x-kubernetes-group-version-kind><- group: node.k8s.io
  kind: RuntimeClassList
  version: v1

Ž
io.k8s.api.node.v1.Schedulingì
éÊobjectúÜ
U
nodeSelectorE
CÊobject‚

ÊstringŠ ¢"
x-kubernetes-map-type	atomic

‚
tolerationss
qÊarrayò@
>
<Ò64
2#/components/schemas/io.k8s.api.core.v1.TolerationŠ ¢#
x-kubernetes-list-type	atomic

O
-io.k8s.apimachinery.pkg.api.resource.Quantity
Ú
	ÊstringÚ
	Ênumber
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
- group: node.k8s.io
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
- group: node.k8s.io
  kind: WatchEvent
  version: v1

;
,io.k8s.apimachinery.pkg.runtime.RawExtension
	Êobject