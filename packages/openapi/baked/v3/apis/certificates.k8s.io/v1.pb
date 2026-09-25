
3.0.0

Kubernetes2v1.36.4+k8flare"Ìê
È
/apis/certificates.k8s.io/v1/¦"£
certificates_v1get available resources*getCertificatesV1APIResourcesB×Ô
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
™¥
7/apis/certificates.k8s.io/v1/certificatesigningrequestsÜ¤"”@
certificates_v17list or watch objects of kind CertificateSigningRequest*+listCertificatesV1CertificateSigningRequest2ª
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
M#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequestList
t
application/json;stream=watchS
QO
M#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequestList
z
#application/vnd.kubernetes.protobufS
QO
M#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequestList
‡
0application/vnd.kubernetes.protobuf;stream=watchS
QO
M#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequestList
g
application/yamlS
QO
M#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequestListj
x-kubernetes-actionlist
jl
x-kubernetes-group-version-kindIGgroup: certificates.k8s.io
version: v1
kind: CertificateSigningRequest
2û
certificates_v1"create a CertificateSigningRequest*-createCertificatesV1CertificateSigningRequest2
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
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequestBÔ
200Ì
É
OKÂ
c
application/jsonO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequestÙ
201Ñ
Î
CreatedÂ
c
application/jsonO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequestÚ
202Ò
Ï
AcceptedÂ
c
application/jsonO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequestj
x-kubernetes-actionpost
jl
x-kubernetes-group-version-kindIGgroup: certificates.k8s.io
version: v1
kind: CertificateSigningRequest
:†L
certificates_v1.delete collection of CertificateSigningRequest*7deleteCertificatesV1CollectionCertificateSigningRequest2î	
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
 Êinteger2Ğ
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
jl
x-kubernetes-group-version-kindIGgroup: certificates.k8s.io
version: v1
kind: CertificateSigningRequest
j»
¸
prettyquery–If 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
 Êstring
ÖO
>/apis/certificates.k8s.io/v1/certificatesigningrequests/{name}“O"Ó
certificates_v1,read the specified CertificateSigningRequest*+readCertificatesV1CertificateSigningRequestB×Ô
200Ì
É
OKÂ
c
application/jsonO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequestj
x-kubernetes-actionget
jl
x-kubernetes-group-version-kindIGgroup: certificates.k8s.io
version: v1
kind: CertificateSigningRequest
*«
certificates_v1/replace the specified CertificateSigningRequest*.replaceCertificatesV1CertificateSigningRequest2
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
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequestB³Ô
200Ì
É
OKÂ
c
application/jsonO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequestÙ
201Ñ
Î
CreatedÂ
c
application/jsonO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequestj
x-kubernetes-actionput
jl
x-kubernetes-group-version-kindIGgroup: certificates.k8s.io
version: v1
kind: CertificateSigningRequest
:•
certificates_v1"delete a CertificateSigningRequest*-deleteCertificatesV1CertificateSigningRequest2
š
dryRunqueryøWhen present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
 Êstring2ã
à
gracePeriodSecondsquery±The duration in seconds before the object should be deleted. Value must be non-negative integer. The value zero indicates delete immediately. If this value is nil, the default grace period for the specified type will be used. Defaults to a per object value if not specified. zero means delete immediately.R
 Êinteger2¨
¥
0ignoreStoreReadErrorWithClusterBreakingPotentialqueryØif set to true, it will trigger an unsafe deletion of the resource in case the normal deletion flow fails with a corrupt object error. A resource is considered corrupt if it can not be retrieved from the underlying storage successfully because of a) its data can not be transformed e.g. decryption failure, or b) it fails to decode into an object. NOTE: unsafe deletion ignores finalizer constraints, skips precondition checks, and removes the object from the storage. WARNING: This may potentially break the cluster if the workload associated with the resource being unsafe-deleted relies on normal deletion flow. Use only if you REALLY know what you are doing. The default value is false, and the user must opt in to enable itR
 Êboolean2Ğ
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
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequestÚ
202Ò
Ï
AcceptedÂ
c
application/jsonO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequestj 
x-kubernetes-action	delete
jl
x-kubernetes-group-version-kindIGgroup: certificates.k8s.io
version: v1
kind: CertificateSigningRequest
Rí
certificates_v18partially update the specified CertificateSigningRequest*,patchCertificatesV1CertificateSigningRequest2
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
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequestÙ
201Ñ
Î
CreatedÂ
c
application/jsonO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequestj
x-kubernetes-actionpatch
jl
x-kubernetes-group-version-kindIGgroup: certificates.k8s.io
version: v1
kind: CertificateSigningRequest
jG
E
namepath%name of the CertificateSigningRequest R
 Êstringj»
¸
prettyquery–If 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
 Êstring
ƒ6
G/apis/certificates.k8s.io/v1/certificatesigningrequests/{name}/approval·5"ç
certificates_v18read approval of the specified CertificateSigningRequest*3readCertificatesV1CertificateSigningRequestApprovalB×Ô
200Ì
É
OKÂ
c
application/jsonO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequestj
x-kubernetes-actionget
jl
x-kubernetes-group-version-kindIGgroup: certificates.k8s.io
version: v1
kind: CertificateSigningRequest
*¿
certificates_v1;replace approval of the specified CertificateSigningRequest*6replaceCertificatesV1CertificateSigningRequestApproval2
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
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequestB³Ô
200Ì
É
OKÂ
c
application/jsonO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequestÙ
201Ñ
Î
CreatedÂ
c
application/jsonO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequestj
x-kubernetes-actionput
jl
x-kubernetes-group-version-kindIGgroup: certificates.k8s.io
version: v1
kind: CertificateSigningRequest
R
certificates_v1Dpartially update approval of the specified CertificateSigningRequest*4patchCertificatesV1CertificateSigningRequestApproval2
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
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequestÙ
201Ñ
Î
CreatedÂ
c
application/jsonO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequestj
x-kubernetes-actionpatch
jl
x-kubernetes-group-version-kindIGgroup: certificates.k8s.io
version: v1
kind: CertificateSigningRequest
jG
E
namepath%name of the CertificateSigningRequest R
 Êstringj»
¸
prettyquery–If 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
 Êstring
õ5
E/apis/certificates.k8s.io/v1/certificatesigningrequests/{name}/status«5"ã
certificates_v16read status of the specified CertificateSigningRequest*1readCertificatesV1CertificateSigningRequestStatusB×Ô
200Ì
É
OKÂ
c
application/jsonO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequestj
x-kubernetes-actionget
jl
x-kubernetes-group-version-kindIGgroup: certificates.k8s.io
version: v1
kind: CertificateSigningRequest
*»
certificates_v19replace status of the specified CertificateSigningRequest*4replaceCertificatesV1CertificateSigningRequestStatus2
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
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequestB³Ô
200Ì
É
OKÂ
c
application/jsonO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequestÙ
201Ñ
Î
CreatedÂ
c
application/jsonO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequestj
x-kubernetes-actionput
jl
x-kubernetes-group-version-kindIGgroup: certificates.k8s.io
version: v1
kind: CertificateSigningRequest
Rı
certificates_v1Bpartially update status of the specified CertificateSigningRequest*2patchCertificatesV1CertificateSigningRequestStatus2
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
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequestÙ
201Ñ
Î
CreatedÂ
c
application/jsonO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequest
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequestj
x-kubernetes-actionpatch
jl
x-kubernetes-group-version-kindIGgroup: certificates.k8s.io
version: v1
kind: CertificateSigningRequest
jG
E
namepath%name of the CertificateSigningRequest R
 Êstringj»
¸
prettyquery–If 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
 Êstring
ÂB
=/apis/certificates.k8s.io/v1/watch/certificatesigningrequests€B"–
certificates_v1…watch individual changes to a list of CertificateSigningRequest. deprecated: use the 'watch' parameter with a list operation instead.*0watchCertificatesV1CertificateSigningRequestListBµ²
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
jl
x-kubernetes-group-version-kindIGgroup: certificates.k8s.io
version: v1
kind: CertificateSigningRequest
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
ÅC
D/apis/certificates.k8s.io/v1/watch/certificatesigningrequests/{name}üB"É
certificates_v1Àwatch changes to an object of kind CertificateSigningRequest. deprecated: use the 'watch' parameter with a list operation instead, filtered to a single item with the 'fieldSelector' parameter.*,watchCertificatesV1CertificateSigningRequestBµ²
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
jl
x-kubernetes-group-version-kindIGgroup: certificates.k8s.io
version: v1
kind: CertificateSigningRequest
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
 ÊintegerjG
E
namepath%name of the CertificateSigningRequest R
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
 Êboolean*¤?
¡?

4io.k8s.api.certificates.v1.CertificateSigningRequestä
áºspecÊobjectúØ
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
M#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequestSpecŠ 
e
status[
YÒSQ
O#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequestStatusŠ ¢r
x-kubernetes-group-version-kindOM- group: certificates.k8s.io
  kind: CertificateSigningRequest
  version: v1

÷
=io.k8s.api.certificates.v1.CertificateSigningRequestConditionµ
²ºtypeºstatusÊobjectú•
X
lastTransitionTimeB@
>#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Time
T
lastUpdateTimeB@
>#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Time

message
	Êstring

reason
	Êstring

status
ÊstringŠ 

type
ÊstringŠ 
É
8io.k8s.api.certificates.v1.CertificateSigningRequestListŒ
‰ºitemsÊobjectúû


apiVersion
	Êstring
m
itemsd
bÊarrayòW
U
SÒMK
I#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequestŠ 

kind
	Êstring
Z
metadataN
LÒFD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaŠ ¢v
x-kubernetes-group-version-kindSQ- group: certificates.k8s.io
  kind: CertificateSigningRequestList
  version: v1

ü
8io.k8s.api.certificates.v1.CertificateSigningRequestSpec¿
¼ºrequestº
signerNameÊobjectú˜
)
expirationSeconds
Êintegeršint32
8
extra/
-Êobject‚!

Êarrayò

ÊstringŠ 
O
groupsE
CÊarrayò

ÊstringŠ ¢#
x-kubernetes-list-type	atomic


request
Êstringšbyte


signerName
ÊstringŠ 

uid
	Êstring
ó
usagesè
åÊarrayò³
°
­Âany
Â
cert sign
Âclient auth
Âcode signing
Âcontent commitment
Â	crl sign
Âdata encipherment
Âdecipher only
Âdigital signature
Âemail protection
Âencipher only
Âipsec end system
Âipsec tunnel
Âipsec user
Âkey agreement
Âkey encipherment
Âmicrosoft sgc
Ânetscape sgc
Âocsp signing
Â	s/mime
Âserver auth
Â
signing
Âtimestamping
ÊstringŠ ¢#
x-kubernetes-list-type	atomic


username
	Êstring
¿
:io.k8s.api.certificates.v1.CertificateSigningRequestStatus€
ıÊobjectúğ
!
certificate
Êstringšbyte
Ê

conditions»
¸Êarrayò`
^
\ÒVT
R#/components/schemas/io.k8s.api.certificates.v1.CertificateSigningRequestConditionŠ ¢'
x-kubernetes-list-map-keys	- type
¢ 
x-kubernetes-list-typemap

“
0io.k8s.apimachinery.pkg.apis.meta.v1.APIResourceŞ
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

ò
2io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptions»
¸Êobjectú‘
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
	Êstring¢–
x-kubernetes-group-version-kindsq- group: ""
  kind: DeleteOptions
  version: v1
- group: certificates.k8s.io
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
ğ
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
ı
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
managedFields
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
Ò
/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEvent
›ºtypeºobjectÊobjectúk
O
objectEC
A#/components/schemas/io.k8s.apimachinery.pkg.runtime.RawExtension

type
ÊstringŠ ¢
x-kubernetes-group-version-kindmk- group: ""
  kind: WatchEvent
  version: v1
- group: certificates.k8s.io
  kind: WatchEvent
  version: v1

;
,io.k8s.apimachinery.pkg.runtime.RawExtension
	Êobject