
3.0.0

Kubernetes2v1.36.4+k8flare"Òß
á
&/apis/flowcontrol.apiserver.k8s.io/v1/¶"³
flowcontrolApiserver_v1get available resources*%getFlowcontrolApiserverV1APIResourcesB×Ô
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
æ¢
1/apis/flowcontrol.apiserver.k8s.io/v1/flowschemas¯¢"¯?
flowcontrolApiserver_v1(list or watch objects of kind FlowSchema*$listFlowcontrolApiserverV1FlowSchema2ª
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
 ÊbooleanB’
200‡
„
OKý
W
application/jsonC
A?
=#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchemaList
d
application/json;stream=watchC
A?
=#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchemaList
j
#application/vnd.kubernetes.protobufC
A?
=#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchemaList
w
0application/vnd.kubernetes.protobuf;stream=watchC
A?
=#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchemaList
W
application/yamlC
A?
=#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchemaListj
x-kubernetes-actionlist
jf
x-kubernetes-group-version-kindCAgroup: flowcontrol.apiserver.k8s.io
version: v1
kind: FlowSchema
2Ç
flowcontrolApiserver_v1create a FlowSchema*&createFlowcontrolApiserverV1FlowSchema2
š
dryRunqueryøWhen present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
 Êstring2•
’
fieldManagerqueryêfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
 Êstring2Û
Ø
fieldValidationquery­fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
 Êstring:N
LH
F
*/*?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchemaB€¤
200œ
™
OK’
S
application/json?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchema
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchema
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchema©
201¡
ž
Created’
S
application/json?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchema
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchema
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchemaª
202¢
Ÿ
Accepted’
S
application/json?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchema
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchema
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchemaj
x-kubernetes-actionpost
jf
x-kubernetes-group-version-kindCAgroup: flowcontrol.apiserver.k8s.io
version: v1
kind: FlowSchema
:òK
flowcontrolApiserver_v1delete collection of FlowSchema*0deleteFlowcontrolApiserverV1CollectionFlowSchema2î	
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
jf
x-kubernetes-group-version-kindCAgroup: flowcontrol.apiserver.k8s.io
version: v1
kind: FlowSchema
j»
¸
prettyquery–If 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
 Êstring
‘L
8/apis/flowcontrol.apiserver.k8s.io/v1/flowschemas/{name}ÔK"
flowcontrolApiserver_v1read the specified FlowSchema*$readFlowcontrolApiserverV1FlowSchemaB§¤
200œ
™
OK’
S
application/json?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchema
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchema
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchemaj
x-kubernetes-actionget
jf
x-kubernetes-group-version-kindCAgroup: flowcontrol.apiserver.k8s.io
version: v1
kind: FlowSchema
*§
flowcontrolApiserver_v1 replace the specified FlowSchema*'replaceFlowcontrolApiserverV1FlowSchema2
š
dryRunqueryøWhen present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
 Êstring2•
’
fieldManagerqueryêfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
 Êstring2Û
Ø
fieldValidationquery­fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
 Êstring:N
LH
F
*/*?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchemaBÓ¤
200œ
™
OK’
S
application/json?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchema
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchema
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchema©
201¡
ž
Created’
S
application/json?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchema
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchema
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchemaj
x-kubernetes-actionput
jf
x-kubernetes-group-version-kindCAgroup: flowcontrol.apiserver.k8s.io
version: v1
kind: FlowSchema
:¡
flowcontrolApiserver_v1delete a FlowSchema*&deleteFlowcontrolApiserverV1FlowSchema2
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
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsBÔ¤
200œ
™
OK’
S
application/json?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchema
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchema
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchemaª
202¢
Ÿ
Accepted’
S
application/json?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchema
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchema
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchemaj 
x-kubernetes-action	delete
jf
x-kubernetes-group-version-kindCAgroup: flowcontrol.apiserver.k8s.io
version: v1
kind: FlowSchema
Rù
flowcontrolApiserver_v1)partially update the specified FlowSchema*%patchFlowcontrolApiserverV1FlowSchema2
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
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchBÓ¤
200œ
™
OK’
S
application/json?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchema
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchema
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchema©
201¡
ž
Created’
S
application/json?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchema
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchema
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchemaj
x-kubernetes-actionpatch
jf
x-kubernetes-group-version-kindCAgroup: flowcontrol.apiserver.k8s.io
version: v1
kind: FlowSchema
j8
6
namepathname of the FlowSchema R
 Êstringj»
¸
prettyquery–If 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
 Êstring
¤3
?/apis/flowcontrol.apiserver.k8s.io/v1/flowschemas/{name}/statusà2"Ÿ
flowcontrolApiserver_v1'read status of the specified FlowSchema**readFlowcontrolApiserverV1FlowSchemaStatusB§¤
200œ
™
OK’
S
application/json?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchema
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchema
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchemaj
x-kubernetes-actionget
jf
x-kubernetes-group-version-kindCAgroup: flowcontrol.apiserver.k8s.io
version: v1
kind: FlowSchema
*·
flowcontrolApiserver_v1*replace status of the specified FlowSchema*-replaceFlowcontrolApiserverV1FlowSchemaStatus2
š
dryRunqueryøWhen present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
 Êstring2•
’
fieldManagerqueryêfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
 Êstring2Û
Ø
fieldValidationquery­fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
 Êstring:N
LH
F
*/*?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchemaBÓ¤
200œ
™
OK’
S
application/json?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchema
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchema
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchema©
201¡
ž
Created’
S
application/json?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchema
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchema
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchemaj
x-kubernetes-actionput
jf
x-kubernetes-group-version-kindCAgroup: flowcontrol.apiserver.k8s.io
version: v1
kind: FlowSchema
R‰
flowcontrolApiserver_v13partially update status of the specified FlowSchema*+patchFlowcontrolApiserverV1FlowSchemaStatus2
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
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchBÓ¤
200œ
™
OK’
S
application/json?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchema
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchema
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchema©
201¡
ž
Created’
S
application/json?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchema
f
#application/vnd.kubernetes.protobuf?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchema
S
application/yaml?
=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchemaj
x-kubernetes-actionpatch
jf
x-kubernetes-group-version-kindCAgroup: flowcontrol.apiserver.k8s.io
version: v1
kind: FlowSchema
j8
6
namepathname of the FlowSchema R
 Êstringj»
¸
prettyquery–If 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
 Êstring
÷¥
A/apis/flowcontrol.apiserver.k8s.io/v1/prioritylevelconfigurations°¥"°@
flowcontrolApiserver_v18list or watch objects of kind PriorityLevelConfiguration*4listFlowcontrolApiserverV1PriorityLevelConfiguration2ª
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
 ÊbooleanBãà
200Ø
Õ
OKÎ
g
application/jsonS
QO
M#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfigurationList
t
application/json;stream=watchS
QO
M#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfigurationList
z
#application/vnd.kubernetes.protobufS
QO
M#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfigurationList
‡
0application/vnd.kubernetes.protobuf;stream=watchS
QO
M#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfigurationList
g
application/yamlS
QO
M#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfigurationListj
x-kubernetes-actionlist
jv
x-kubernetes-group-version-kindSQgroup: flowcontrol.apiserver.k8s.io
version: v1
kind: PriorityLevelConfiguration
2—
flowcontrolApiserver_v1#create a PriorityLevelConfiguration*6createFlowcontrolApiserverV1PriorityLevelConfiguration2
š
dryRunqueryøWhen present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
 Êstring2•
’
fieldManagerqueryêfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
 Êstring2Û
Ø
fieldValidationquery­fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
 Êstring:^
\X
V
*/*O
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfigurationBÔ
200Ì
É
OKÂ
c
application/jsonO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfiguration
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfiguration
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfigurationÙ
201Ñ
Î
CreatedÂ
c
application/jsonO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfiguration
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfiguration
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfigurationÚ
202Ò
Ï
AcceptedÂ
c
application/jsonO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfiguration
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfiguration
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfigurationj
x-kubernetes-actionpost
jv
x-kubernetes-group-version-kindSQgroup: flowcontrol.apiserver.k8s.io
version: v1
kind: PriorityLevelConfiguration
:¢L
flowcontrolApiserver_v1/delete collection of PriorityLevelConfiguration*@deleteFlowcontrolApiserverV1CollectionPriorityLevelConfiguration2î	
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
jv
x-kubernetes-group-version-kindSQgroup: flowcontrol.apiserver.k8s.io
version: v1
kind: PriorityLevelConfiguration
j»
¸
prettyquery–If 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
 Êstring
ÑP
H/apis/flowcontrol.apiserver.k8s.io/v1/prioritylevelconfigurations/{name}„P"ï
flowcontrolApiserver_v1-read the specified PriorityLevelConfiguration*4readFlowcontrolApiserverV1PriorityLevelConfigurationB×Ô
200Ì
É
OKÂ
c
application/jsonO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfiguration
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfiguration
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfigurationj
x-kubernetes-actionget
jv
x-kubernetes-group-version-kindSQgroup: flowcontrol.apiserver.k8s.io
version: v1
kind: PriorityLevelConfiguration
*Ç
flowcontrolApiserver_v10replace the specified PriorityLevelConfiguration*7replaceFlowcontrolApiserverV1PriorityLevelConfiguration2
š
dryRunqueryøWhen present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
 Êstring2•
’
fieldManagerqueryêfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
 Êstring2Û
Ø
fieldValidationquery­fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
 Êstring:^
\X
V
*/*O
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfigurationB³Ô
200Ì
É
OKÂ
c
application/jsonO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfiguration
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfiguration
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfigurationÙ
201Ñ
Î
CreatedÂ
c
application/jsonO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfiguration
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfiguration
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfigurationj
x-kubernetes-actionput
jv
x-kubernetes-group-version-kindSQgroup: flowcontrol.apiserver.k8s.io
version: v1
kind: PriorityLevelConfiguration
:±
flowcontrolApiserver_v1#delete a PriorityLevelConfiguration*6deleteFlowcontrolApiserverV1PriorityLevelConfiguration2
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
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsB´Ô
200Ì
É
OKÂ
c
application/jsonO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfiguration
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfiguration
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfigurationÚ
202Ò
Ï
AcceptedÂ
c
application/jsonO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfiguration
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfiguration
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfigurationj 
x-kubernetes-action	delete
jv
x-kubernetes-group-version-kindSQgroup: flowcontrol.apiserver.k8s.io
version: v1
kind: PriorityLevelConfiguration
R‰
flowcontrolApiserver_v19partially update the specified PriorityLevelConfiguration*5patchFlowcontrolApiserverV1PriorityLevelConfiguration2
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
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchB³Ô
200Ì
É
OKÂ
c
application/jsonO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfiguration
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfiguration
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfigurationÙ
201Ñ
Î
CreatedÂ
c
application/jsonO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfiguration
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfiguration
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfigurationj
x-kubernetes-actionpatch
jv
x-kubernetes-group-version-kindSQgroup: flowcontrol.apiserver.k8s.io
version: v1
kind: PriorityLevelConfiguration
jH
F
namepath&name of the PriorityLevelConfiguration R
 Êstringj»
¸
prettyquery–If 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
 Êstring
Ô6
O/apis/flowcontrol.apiserver.k8s.io/v1/prioritylevelconfigurations/{name}/status€6"ÿ
flowcontrolApiserver_v17read status of the specified PriorityLevelConfiguration*:readFlowcontrolApiserverV1PriorityLevelConfigurationStatusB×Ô
200Ì
É
OKÂ
c
application/jsonO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfiguration
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfiguration
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfigurationj
x-kubernetes-actionget
jv
x-kubernetes-group-version-kindSQgroup: flowcontrol.apiserver.k8s.io
version: v1
kind: PriorityLevelConfiguration
*×
flowcontrolApiserver_v1:replace status of the specified PriorityLevelConfiguration*=replaceFlowcontrolApiserverV1PriorityLevelConfigurationStatus2
š
dryRunqueryøWhen present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
 Êstring2•
’
fieldManagerqueryêfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
 Êstring2Û
Ø
fieldValidationquery­fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
 Êstring:^
\X
V
*/*O
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfigurationB³Ô
200Ì
É
OKÂ
c
application/jsonO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfiguration
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfiguration
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfigurationÙ
201Ñ
Î
CreatedÂ
c
application/jsonO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfiguration
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfiguration
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfigurationj
x-kubernetes-actionput
jv
x-kubernetes-group-version-kindSQgroup: flowcontrol.apiserver.k8s.io
version: v1
kind: PriorityLevelConfiguration
R™
flowcontrolApiserver_v1Cpartially update status of the specified PriorityLevelConfiguration*;patchFlowcontrolApiserverV1PriorityLevelConfigurationStatus2
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
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchB³Ô
200Ì
É
OKÂ
c
application/jsonO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfiguration
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfiguration
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfigurationÙ
201Ñ
Î
CreatedÂ
c
application/jsonO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfiguration
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfiguration
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfigurationj
x-kubernetes-actionpatch
jv
x-kubernetes-group-version-kindSQgroup: flowcontrol.apiserver.k8s.io
version: v1
kind: PriorityLevelConfiguration
jH
F
namepath&name of the PriorityLevelConfiguration R
 Êstringj»
¸
prettyquery–If 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
 Êstring
§B
7/apis/flowcontrol.apiserver.k8s.io/v1/watch/flowschemasëA"
flowcontrolApiserver_v1vwatch individual changes to a list of FlowSchema. deprecated: use the 'watch' parameter with a list operation instead.*)watchFlowcontrolApiserverV1FlowSchemaListBµ²
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
jf
x-kubernetes-group-version-kindCAgroup: flowcontrol.apiserver.k8s.io
version: v1
kind: FlowSchema
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
œC
>/apis/flowcontrol.apiserver.k8s.io/v1/watch/flowschemas/{name}ÙB"µ
flowcontrolApiserver_v1±watch changes to an object of kind FlowSchema. deprecated: use the 'watch' parameter with a list operation instead, filtered to a single item with the 'fieldSelector' parameter.*%watchFlowcontrolApiserverV1FlowSchemaBµ²
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
jf
x-kubernetes-group-version-kindCAgroup: flowcontrol.apiserver.k8s.io
version: v1
kind: FlowSchema
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
 Êintegerj8
6
namepathname of the FlowSchema R
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
èB
G/apis/flowcontrol.apiserver.k8s.io/v1/watch/prioritylevelconfigurationsœB"²
flowcontrolApiserver_v1†watch individual changes to a list of PriorityLevelConfiguration. deprecated: use the 'watch' parameter with a list operation instead.*9watchFlowcontrolApiserverV1PriorityLevelConfigurationListBµ²
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
jv
x-kubernetes-group-version-kindSQgroup: flowcontrol.apiserver.k8s.io
version: v1
kind: PriorityLevelConfiguration
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
ìC
N/apis/flowcontrol.apiserver.k8s.io/v1/watch/prioritylevelconfigurations/{name}™C"å
flowcontrolApiserver_v1Áwatch changes to an object of kind PriorityLevelConfiguration. deprecated: use the 'watch' parameter with a list operation instead, filtered to a single item with the 'fieldSelector' parameter.*5watchFlowcontrolApiserverV1PriorityLevelConfigurationBµ²
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
jv
x-kubernetes-group-version-kindSQgroup: flowcontrol.apiserver.k8s.io
version: v1
kind: PriorityLevelConfiguration
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
 ÊintegerjH
F
namepath&name of the PriorityLevelConfiguration R
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
 Êboolean*«d
¨d
§
:io.k8s.api.flowcontrol.v1.ExemptPriorityLevelConfigurationi
gÊobjectú[
'
lendablePercent
Êintegeršint32
0
nominalConcurrencyShares
Êintegeršint32
d
1io.k8s.api.flowcontrol.v1.FlowDistinguisherMethod/
-ºtypeÊobjectú

type
ÊstringŠ 
à
$io.k8s.api.flowcontrol.v1.FlowSchema·
´Êobjectú¸
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
Q
specI
GÒA?
=#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchemaSpecŠ 
U
statusK
IÒCA
?#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchemaStatusŠ ¢l
x-kubernetes-group-version-kindIG- group: flowcontrol.apiserver.k8s.io
  kind: FlowSchema
  version: v1

÷
-io.k8s.api.flowcontrol.v1.FlowSchemaConditionÅ
ÂÊobjectúµ
X
lastTransitionTimeB@
>#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Time

message
	Êstring

reason
	Êstring

status
	Êstring

type
	Êstring
£
(io.k8s.api.flowcontrol.v1.FlowSchemaListö
óºitemsÊobjectúë


apiVersion
	Êstring
]
itemsT
RÊarrayòG
E
CÒ=;
9#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchemaŠ 

kind
	Êstring
Z
metadataN
LÒFD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaŠ ¢p
x-kubernetes-group-version-kindMK- group: flowcontrol.apiserver.k8s.io
  kind: FlowSchemaList
  version: v1

ˆ
(io.k8s.api.flowcontrol.v1.FlowSchemaSpecÛ
ØºpriorityLevelConfigurationÊobjectú®
a
distinguisherMethodJH
F#/components/schemas/io.k8s.api.flowcontrol.v1.FlowDistinguisherMethod
6
matchingPrecedence 
ÊintegerŠ		        šint32
|
priorityLevelConfiguration^
\ÒVT
R#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfigurationReferenceŠ 
’
rulesˆ
…ÊarrayòT
R
PÒJH
F#/components/schemas/io.k8s.api.flowcontrol.v1.PolicyRulesWithSubjectsŠ ¢#
x-kubernetes-list-type	atomic

Ð
*io.k8s.api.flowcontrol.v1.FlowSchemaStatus¡
žÊobjectú‘
Ž

conditionsÿ
üÊarrayòP
N
LÒFD
B#/components/schemas/io.k8s.api.flowcontrol.v1.FlowSchemaConditionŠ ¢'
x-kubernetes-list-map-keys	- type
¢ 
x-kubernetes-list-typemap
¢'
x-kubernetes-patch-merge-keytype
¢'
x-kubernetes-patch-strategymerge

Y
&io.k8s.api.flowcontrol.v1.GroupSubject/
-ºnameÊobjectú

name
ÊstringŠ 
“
'io.k8s.api.flowcontrol.v1.LimitResponseç
äºtypeÊobjectún
R
queuingGE
C#/components/schemas/io.k8s.api.flowcontrol.v1.QueuingConfiguration

type
ÊstringŠ ¢`
x-kubernetes-unionsIG- discriminator: type
  fields-to-discriminateBy:
    queuing: Queuing

µ
;io.k8s.api.flowcontrol.v1.LimitedPriorityLevelConfigurationõ
òÊobjectúå
-
borrowingLimitPercent
Êintegeršint32
'
lendablePercent
Êintegeršint32
Y
limitResponseH
FÒ@>
<#/components/schemas/io.k8s.api.flowcontrol.v1.LimitResponseŠ 
0
nominalConcurrencyShares
Êintegeršint32
‚
/io.k8s.api.flowcontrol.v1.NonResourcePolicyRuleÎ
ËºverbsºnonResourceURLsÊobjectú¤
U
nonResourceURLsB
@Êarrayò

ÊstringŠ ¢ 
x-kubernetes-list-typeset

K
verbsB
@Êarrayò

ÊstringŠ ¢ 
x-kubernetes-list-typeset


1io.k8s.api.flowcontrol.v1.PolicyRulesWithSubjects×
ÔºsubjectsÊobjectú¼
›
nonResourceRules†
ƒÊarrayòR
P
NÒHF
D#/components/schemas/io.k8s.api.flowcontrol.v1.NonResourcePolicyRuleŠ ¢#
x-kubernetes-list-type	atomic

•
resourceRulesƒ
€ÊarrayòO
M
KÒEC
A#/components/schemas/io.k8s.api.flowcontrol.v1.ResourcePolicyRuleŠ ¢#
x-kubernetes-list-type	atomic

ƒ
subjectsw
uÊarrayòD
B
@Ò:8
6#/components/schemas/io.k8s.api.flowcontrol.v1.SubjectŠ ¢#
x-kubernetes-list-type	atomic

 
4io.k8s.api.flowcontrol.v1.PriorityLevelConfigurationç
äÊobjectúØ
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
a
specY
WÒQO
M#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfigurationSpecŠ 
e
status[
YÒSQ
O#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfigurationStatusŠ ¢|
x-kubernetes-group-version-kindYW- group: flowcontrol.apiserver.k8s.io
  kind: PriorityLevelConfiguration
  version: v1

‡
=io.k8s.api.flowcontrol.v1.PriorityLevelConfigurationConditionÅ
ÂÊobjectúµ
X
lastTransitionTimeB@
>#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Time

message
	Êstring

reason
	Êstring

status
	Êstring

type
	Êstring
Ô
8io.k8s.api.flowcontrol.v1.PriorityLevelConfigurationList—
”ºitemsÊobjectúû


apiVersion
	Êstring
m
itemsd
bÊarrayòW
U
SÒMK
I#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfigurationŠ 

kind
	Êstring
Z
metadataN
LÒFD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaŠ ¢€
x-kubernetes-group-version-kind][- group: flowcontrol.apiserver.k8s.io
  kind: PriorityLevelConfigurationList
  version: v1

p
=io.k8s.api.flowcontrol.v1.PriorityLevelConfigurationReference/
-ºnameÊobjectú

name
ÊstringŠ 
¤
8io.k8s.api.flowcontrol.v1.PriorityLevelConfigurationSpecç
äºtypeÊobjectúÚ
]
exemptSQ
O#/components/schemas/io.k8s.api.flowcontrol.v1.ExemptPriorityLevelConfiguration
_
limitedTR
P#/components/schemas/io.k8s.api.flowcontrol.v1.LimitedPriorityLevelConfiguration

type
ÊstringŠ ¢s
x-kubernetes-unions\Z- discriminator: type
  fields-to-discriminateBy:
    exempt: Exempt
    limited: Limited

ð
:io.k8s.api.flowcontrol.v1.PriorityLevelConfigurationStatus±
®Êobjectú¡
ž

conditions
ŒÊarrayò`
^
\ÒVT
R#/components/schemas/io.k8s.api.flowcontrol.v1.PriorityLevelConfigurationConditionŠ ¢'
x-kubernetes-list-map-keys	- type
¢ 
x-kubernetes-list-typemap
¢'
x-kubernetes-patch-merge-keytype
¢'
x-kubernetes-patch-strategymerge

Ó
.io.k8s.api.flowcontrol.v1.QueuingConfiguration 
Êobjectú
,
handSize 
ÊintegerŠ		        šint32
4
queueLengthLimit 
ÊintegerŠ		        šint32
*
queues 
ÊintegerŠ		        šint32
À
,io.k8s.api.flowcontrol.v1.ResourcePolicyRule
Œºverbsº	apiGroupsº	resourcesÊobjectúß
O
	apiGroupsB
@Êarrayò

ÊstringŠ ¢ 
x-kubernetes-list-typeset


clusterScope

Êboolean
P

namespacesB
@Êarrayò

ÊstringŠ ¢ 
x-kubernetes-list-typeset

O
	resourcesB
@Êarrayò

ÊstringŠ ¢ 
x-kubernetes-list-typeset

K
verbsB
@Êarrayò

ÊstringŠ ¢ 
x-kubernetes-list-typeset


/io.k8s.api.flowcontrol.v1.ServiceAccountSubjectZ
Xº	namespaceºnameÊobjectú9

name
ÊstringŠ 

	namespace
ÊstringŠ 
×
!io.k8s.api.flowcontrol.v1.Subject±
®ºkindÊobjectúˆ
H
group?=
;#/components/schemas/io.k8s.api.flowcontrol.v1.GroupSubject

kind
ÊstringŠ 
Z
serviceAccountHF
D#/components/schemas/io.k8s.api.flowcontrol.v1.ServiceAccountSubject
F
user><
:#/components/schemas/io.k8s.api.flowcontrol.v1.UserSubject¢Ž
x-kubernetes-unionswu- discriminator: kind
  fields-to-discriminateBy:
    group: Group
    serviceAccount: ServiceAccount
    user: User

X
%io.k8s.api.flowcontrol.v1.UserSubject/
-ºnameÊobjectú

name
ÊstringŠ 
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

û
2io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsÄ
ÁÊobjectú‘
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
	Êstring¢Ÿ
x-kubernetes-group-version-kind|z- group: ""
  kind: DeleteOptions
  version: v1
- group: flowcontrol.apiserver.k8s.io
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
Û
/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent§
¤ºtypeºobjectÊobjectúk
O
objectEC
A#/components/schemas/io.k8s.apimachinery.pkg.runtime.RawExtension

type
ÊstringŠ ¢™
x-kubernetes-group-version-kindvt- group: ""
  kind: WatchEvent
  version: v1
- group: flowcontrol.apiserver.k8s.io
  kind: WatchEvent
  version: v1

;
,io.k8s.apimachinery.pkg.runtime.RawExtension
	Êobject