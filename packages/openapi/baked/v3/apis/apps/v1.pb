
3.0.0

Kubernetes2v1.36.4+k8flare"ÿØ
©
/apis/apps/v1/ñ"ì
apps_v1get available resources*getAppsV1APIResourcesB◊‘
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
ãA
!/apis/apps/v1/controllerrevisionsÂ@"˚
apps_v10list or watch objects of kind ControllerRevision*,listAppsV1ControllerRevisionForAllNamespacesBóî
200å
â
OKÇ
X
application/jsonD
B@
>#/components/schemas/io.k8s.api.apps.v1.ControllerRevisionList
e
application/json;stream=watchD
B@
>#/components/schemas/io.k8s.api.apps.v1.ControllerRevisionList
k
#application/vnd.kubernetes.protobufD
B@
>#/components/schemas/io.k8s.api.apps.v1.ControllerRevisionList
x
0application/vnd.kubernetes.protobuf;stream=watchD
B@
>#/components/schemas/io.k8s.api.apps.v1.ControllerRevisionList
X
application/yamlD
B@
>#/components/schemas/io.k8s.api.apps.v1.ControllerRevisionListj
x-kubernetes-actionlist
jV
x-kubernetes-group-version-kind31group: apps
version: v1
kind: ControllerRevision
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
∫@
/apis/apps/v1/daemonsetsù@"≥
apps_v1'list or watch objects of kind DaemonSet*#listAppsV1DaemonSetForAllNamespacesBÍÁ
200ﬂ
‹
OK’
O
application/json;
97
5#/components/schemas/io.k8s.api.apps.v1.DaemonSetList
\
application/json;stream=watch;
97
5#/components/schemas/io.k8s.api.apps.v1.DaemonSetList
b
#application/vnd.kubernetes.protobuf;
97
5#/components/schemas/io.k8s.api.apps.v1.DaemonSetList
o
0application/vnd.kubernetes.protobuf;stream=watch;
97
5#/components/schemas/io.k8s.api.apps.v1.DaemonSetList
O
application/yaml;
97
5#/components/schemas/io.k8s.api.apps.v1.DaemonSetListj
x-kubernetes-actionlist
jM
x-kubernetes-group-version-kind*(group: apps
version: v1
kind: DaemonSet
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
√@
/apis/apps/v1/deployments•@"ª
apps_v1(list or watch objects of kind Deployment*$listAppsV1DeploymentForAllNamespacesBÔÏ
200‰
·
OK⁄
P
application/json<
:8
6#/components/schemas/io.k8s.api.apps.v1.DeploymentList
]
application/json;stream=watch<
:8
6#/components/schemas/io.k8s.api.apps.v1.DeploymentList
c
#application/vnd.kubernetes.protobuf<
:8
6#/components/schemas/io.k8s.api.apps.v1.DeploymentList
p
0application/vnd.kubernetes.protobuf;stream=watch<
:8
6#/components/schemas/io.k8s.api.apps.v1.DeploymentList
P
application/yaml<
:8
6#/components/schemas/io.k8s.api.apps.v1.DeploymentListj
x-kubernetes-actionlist
jN
x-kubernetes-group-version-kind+)group: apps
version: v1
kind: Deployment
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
ù£
8/apis/apps/v1/namespaces/{namespace}/controllerrevisionsﬂ¢"û?
apps_v10list or watch objects of kind ControllerRevision*&listAppsV1NamespacedControllerRevision2™
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
† booleanBóî
200å
â
OKÇ
X
application/jsonD
B@
>#/components/schemas/io.k8s.api.apps.v1.ControllerRevisionList
e
application/json;stream=watchD
B@
>#/components/schemas/io.k8s.api.apps.v1.ControllerRevisionList
k
#application/vnd.kubernetes.protobufD
B@
>#/components/schemas/io.k8s.api.apps.v1.ControllerRevisionList
x
0application/vnd.kubernetes.protobuf;stream=watchD
B@
>#/components/schemas/io.k8s.api.apps.v1.ControllerRevisionList
X
application/yamlD
B@
>#/components/schemas/io.k8s.api.apps.v1.ControllerRevisionListj
x-kubernetes-actionlist
jV
x-kubernetes-group-version-kind31group: apps
version: v1
kind: ControllerRevision
2ª
apps_v1create a ControllerRevision*(createAppsV1NamespacedControllerRevision2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:O
MI
G
*/*@
><
:#/components/schemas/io.k8s.api.apps.v1.ControllerRevisionBâß
200ü
ú
OKï
T
application/json@
><
:#/components/schemas/io.k8s.api.apps.v1.ControllerRevision
g
#application/vnd.kubernetes.protobuf@
><
:#/components/schemas/io.k8s.api.apps.v1.ControllerRevision
T
application/yaml@
><
:#/components/schemas/io.k8s.api.apps.v1.ControllerRevision¨
201§
°
Createdï
T
application/json@
><
:#/components/schemas/io.k8s.api.apps.v1.ControllerRevision
g
#application/vnd.kubernetes.protobuf@
><
:#/components/schemas/io.k8s.api.apps.v1.ControllerRevision
T
application/yaml@
><
:#/components/schemas/io.k8s.api.apps.v1.ControllerRevision≠
202•
¢
Acceptedï
T
application/json@
><
:#/components/schemas/io.k8s.api.apps.v1.ControllerRevision
g
#application/vnd.kubernetes.protobuf@
><
:#/components/schemas/io.k8s.api.apps.v1.ControllerRevision
T
application/yaml@
><
:#/components/schemas/io.k8s.api.apps.v1.ControllerRevisionj
x-kubernetes-actionpost
jV
x-kubernetes-group-version-kind31group: apps
version: v1
kind: ControllerRevision
:‹K
apps_v1'delete collection of ControllerRevision*2deleteAppsV1CollectionNamespacedControllerRevision2Ó	
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
jV
x-kubernetes-group-version-kind31group: apps
version: v1
kind: ControllerRevision
ja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
¡L
?/apis/apps/v1/namespaces/{namespace}/controllerrevisions/{name}˝K"¸
apps_v1%read the specified ControllerRevision*&readAppsV1NamespacedControllerRevisionB™ß
200ü
ú
OKï
T
application/json@
><
:#/components/schemas/io.k8s.api.apps.v1.ControllerRevision
g
#application/vnd.kubernetes.protobuf@
><
:#/components/schemas/io.k8s.api.apps.v1.ControllerRevision
T
application/yaml@
><
:#/components/schemas/io.k8s.api.apps.v1.ControllerRevisionj
x-kubernetes-actionget
jV
x-kubernetes-group-version-kind31group: apps
version: v1
kind: ControllerRevision
*ò
apps_v1(replace the specified ControllerRevision*)replaceAppsV1NamespacedControllerRevision2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:O
MI
G
*/*@
><
:#/components/schemas/io.k8s.api.apps.v1.ControllerRevisionBŸß
200ü
ú
OKï
T
application/json@
><
:#/components/schemas/io.k8s.api.apps.v1.ControllerRevision
g
#application/vnd.kubernetes.protobuf@
><
:#/components/schemas/io.k8s.api.apps.v1.ControllerRevision
T
application/yaml@
><
:#/components/schemas/io.k8s.api.apps.v1.ControllerRevision¨
201§
°
Createdï
T
application/json@
><
:#/components/schemas/io.k8s.api.apps.v1.ControllerRevision
g
#application/vnd.kubernetes.protobuf@
><
:#/components/schemas/io.k8s.api.apps.v1.ControllerRevision
T
application/yaml@
><
:#/components/schemas/io.k8s.api.apps.v1.ControllerRevisionj
x-kubernetes-actionput
jV
x-kubernetes-group-version-kind31group: apps
version: v1
kind: ControllerRevision
:ë
apps_v1delete a ControllerRevision*(deleteAppsV1NamespacedControllerRevision2ù
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
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsB⁄ß
200ü
ú
OKï
T
application/json@
><
:#/components/schemas/io.k8s.api.apps.v1.ControllerRevision
g
#application/vnd.kubernetes.protobuf@
><
:#/components/schemas/io.k8s.api.apps.v1.ControllerRevision
T
application/yaml@
><
:#/components/schemas/io.k8s.api.apps.v1.ControllerRevision≠
202•
¢
Acceptedï
T
application/json@
><
:#/components/schemas/io.k8s.api.apps.v1.ControllerRevision
g
#application/vnd.kubernetes.protobuf@
><
:#/components/schemas/io.k8s.api.apps.v1.ControllerRevision
T
application/yaml@
><
:#/components/schemas/io.k8s.api.apps.v1.ControllerRevisionj 
x-kubernetes-action	delete
jV
x-kubernetes-group-version-kind31group: apps
version: v1
kind: ControllerRevision
RÈ
apps_v11partially update the specified ControllerRevision*'patchAppsV1NamespacedControllerRevision2ù
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
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchBŸß
200ü
ú
OKï
T
application/json@
><
:#/components/schemas/io.k8s.api.apps.v1.ControllerRevision
g
#application/vnd.kubernetes.protobuf@
><
:#/components/schemas/io.k8s.api.apps.v1.ControllerRevision
T
application/yaml@
><
:#/components/schemas/io.k8s.api.apps.v1.ControllerRevision¨
201§
°
Createdï
T
application/json@
><
:#/components/schemas/io.k8s.api.apps.v1.ControllerRevision
g
#application/vnd.kubernetes.protobuf@
><
:#/components/schemas/io.k8s.api.apps.v1.ControllerRevision
T
application/yaml@
><
:#/components/schemas/io.k8s.api.apps.v1.ControllerRevisionj
x-kubernetes-actionpatch
jV
x-kubernetes-group-version-kind31group: apps
version: v1
kind: ControllerRevision
j@
>
namepathname of the ControllerRevision R
† stringja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
º°
//apis/apps/v1/namespaces/{namespace}/daemonsetsá°"÷>
apps_v1'list or watch objects of kind DaemonSet*listAppsV1NamespacedDaemonSet2™
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
† booleanBÍÁ
200ﬂ
‹
OK’
O
application/json;
97
5#/components/schemas/io.k8s.api.apps.v1.DaemonSetList
\
application/json;stream=watch;
97
5#/components/schemas/io.k8s.api.apps.v1.DaemonSetList
b
#application/vnd.kubernetes.protobuf;
97
5#/components/schemas/io.k8s.api.apps.v1.DaemonSetList
o
0application/vnd.kubernetes.protobuf;stream=watch;
97
5#/components/schemas/io.k8s.api.apps.v1.DaemonSetList
O
application/yaml;
97
5#/components/schemas/io.k8s.api.apps.v1.DaemonSetListj
x-kubernetes-actionlist
jM
x-kubernetes-group-version-kind*(group: apps
version: v1
kind: DaemonSet
2∆
apps_v1create a DaemonSet*createAppsV1NamespacedDaemonSet2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:F
D@
>
*/*7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSetB∏å
200Ñ
Å
OK˙
K
application/json7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSet
^
#application/vnd.kubernetes.protobuf7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSet
K
application/yaml7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSetë
201â
Ü
Created˙
K
application/json7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSet
^
#application/vnd.kubernetes.protobuf7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSet
K
application/yaml7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSetí
202ä
á
Accepted˙
K
application/json7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSet
^
#application/vnd.kubernetes.protobuf7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSet
K
application/yaml7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSetj
x-kubernetes-actionpost
jM
x-kubernetes-group-version-kind*(group: apps
version: v1
kind: DaemonSet
:¡K
apps_v1delete collection of DaemonSet*)deleteAppsV1CollectionNamespacedDaemonSet2Ó	
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
jM
x-kubernetes-group-version-kind*(group: apps
version: v1
kind: DaemonSet
ja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
˝I
6/apis/apps/v1/namespaces/{namespace}/daemonsets/{name}¬I"∆
apps_v1read the specified DaemonSet*readAppsV1NamespacedDaemonSetBèå
200Ñ
Å
OK˙
K
application/json7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSet
^
#application/vnd.kubernetes.protobuf7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSet
K
application/yaml7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSetj
x-kubernetes-actionget
jM
x-kubernetes-group-version-kind*(group: apps
version: v1
kind: DaemonSet
*æ
apps_v1replace the specified DaemonSet* replaceAppsV1NamespacedDaemonSet2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:F
D@
>
*/*7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSetB£å
200Ñ
Å
OK˙
K
application/json7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSet
^
#application/vnd.kubernetes.protobuf7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSet
K
application/yaml7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSetë
201â
Ü
Created˙
K
application/json7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSet
^
#application/vnd.kubernetes.protobuf7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSet
K
application/yaml7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSetj
x-kubernetes-actionput
jM
x-kubernetes-group-version-kind*(group: apps
version: v1
kind: DaemonSet
:¿
apps_v1delete a DaemonSet*deleteAppsV1NamespacedDaemonSet2ù
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
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsB§å
200Ñ
Å
OK˙
K
application/json7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSet
^
#application/vnd.kubernetes.protobuf7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSet
K
application/yaml7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSetí
202ä
á
Accepted˙
K
application/json7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSet
^
#application/vnd.kubernetes.protobuf7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSet
K
application/yaml7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSetj 
x-kubernetes-action	delete
jM
x-kubernetes-group-version-kind*(group: apps
version: v1
kind: DaemonSet
Rò
apps_v1(partially update the specified DaemonSet*patchAppsV1NamespacedDaemonSet2ù
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
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchB£å
200Ñ
Å
OK˙
K
application/json7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSet
^
#application/vnd.kubernetes.protobuf7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSet
K
application/yaml7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSetë
201â
Ü
Created˙
K
application/json7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSet
^
#application/vnd.kubernetes.protobuf7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSet
K
application/yaml7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSetj
x-kubernetes-actionpatch
jM
x-kubernetes-group-version-kind*(group: apps
version: v1
kind: DaemonSet
j7
5
namepathname of the DaemonSet R
† stringja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
Ò1
=/apis/apps/v1/namespaces/{namespace}/daemonsets/{name}/statusØ1"÷
apps_v1&read status of the specified DaemonSet*#readAppsV1NamespacedDaemonSetStatusBèå
200Ñ
Å
OK˙
K
application/json7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSet
^
#application/vnd.kubernetes.protobuf7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSet
K
application/yaml7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSetj
x-kubernetes-actionget
jM
x-kubernetes-group-version-kind*(group: apps
version: v1
kind: DaemonSet
*Œ
apps_v1)replace status of the specified DaemonSet*&replaceAppsV1NamespacedDaemonSetStatus2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:F
D@
>
*/*7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSetB£å
200Ñ
Å
OK˙
K
application/json7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSet
^
#application/vnd.kubernetes.protobuf7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSet
K
application/yaml7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSetë
201â
Ü
Created˙
K
application/json7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSet
^
#application/vnd.kubernetes.protobuf7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSet
K
application/yaml7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSetj
x-kubernetes-actionput
jM
x-kubernetes-group-version-kind*(group: apps
version: v1
kind: DaemonSet
R®
apps_v12partially update status of the specified DaemonSet*$patchAppsV1NamespacedDaemonSetStatus2ù
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
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchB£å
200Ñ
Å
OK˙
K
application/json7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSet
^
#application/vnd.kubernetes.protobuf7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSet
K
application/yaml7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSetë
201â
Ü
Created˙
K
application/json7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSet
^
#application/vnd.kubernetes.protobuf7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSet
K
application/yaml7
53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSetj
x-kubernetes-actionpatch
jM
x-kubernetes-group-version-kind*(group: apps
version: v1
kind: DaemonSet
j7
5
namepathname of the DaemonSet R
† stringja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
’°
0/apis/apps/v1/namespaces/{namespace}/deploymentsü°"ﬁ>
apps_v1(list or watch objects of kind Deployment*listAppsV1NamespacedDeployment2™
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
6#/components/schemas/io.k8s.api.apps.v1.DeploymentList
]
application/json;stream=watch<
:8
6#/components/schemas/io.k8s.api.apps.v1.DeploymentList
c
#application/vnd.kubernetes.protobuf<
:8
6#/components/schemas/io.k8s.api.apps.v1.DeploymentList
p
0application/vnd.kubernetes.protobuf;stream=watch<
:8
6#/components/schemas/io.k8s.api.apps.v1.DeploymentList
P
application/yaml<
:8
6#/components/schemas/io.k8s.api.apps.v1.DeploymentListj
x-kubernetes-actionlist
jN
x-kubernetes-group-version-kind+)group: apps
version: v1
kind: Deployment
2”
apps_v1create a Deployment* createAppsV1NamespacedDeployment2ù
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
2#/components/schemas/io.k8s.api.apps.v1.DeploymentB¡è
200á
Ñ
OK˝
L
application/json8
64
2#/components/schemas/io.k8s.api.apps.v1.Deployment
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.apps.v1.Deployment
L
application/yaml8
64
2#/components/schemas/io.k8s.api.apps.v1.Deploymentî
201å
â
Created˝
L
application/json8
64
2#/components/schemas/io.k8s.api.apps.v1.Deployment
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.apps.v1.Deployment
L
application/yaml8
64
2#/components/schemas/io.k8s.api.apps.v1.Deploymentï
202ç
ä
Accepted˝
L
application/json8
64
2#/components/schemas/io.k8s.api.apps.v1.Deployment
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.apps.v1.Deployment
L
application/yaml8
64
2#/components/schemas/io.k8s.api.apps.v1.Deploymentj
x-kubernetes-actionpost
jN
x-kubernetes-group-version-kind+)group: apps
version: v1
kind: Deployment
:ƒK
apps_v1delete collection of Deployment**deleteAppsV1CollectionNamespacedDeployment2Ó	
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
jN
x-kubernetes-group-version-kind+)group: apps
version: v1
kind: Deployment
ja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
°J
7/apis/apps/v1/namespaces/{namespace}/deployments/{name}ÂI"Ã
apps_v1read the specified Deployment*readAppsV1NamespacedDeploymentBíè
200á
Ñ
OK˝
L
application/json8
64
2#/components/schemas/io.k8s.api.apps.v1.Deployment
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.apps.v1.Deployment
L
application/yaml8
64
2#/components/schemas/io.k8s.api.apps.v1.Deploymentj
x-kubernetes-actionget
jN
x-kubernetes-group-version-kind+)group: apps
version: v1
kind: Deployment
*»
apps_v1 replace the specified Deployment*!replaceAppsV1NamespacedDeployment2ù
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
2#/components/schemas/io.k8s.api.apps.v1.DeploymentB©è
200á
Ñ
OK˝
L
application/json8
64
2#/components/schemas/io.k8s.api.apps.v1.Deployment
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.apps.v1.Deployment
L
application/yaml8
64
2#/components/schemas/io.k8s.api.apps.v1.Deploymentî
201å
â
Created˝
L
application/json8
64
2#/components/schemas/io.k8s.api.apps.v1.Deployment
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.apps.v1.Deployment
L
application/yaml8
64
2#/components/schemas/io.k8s.api.apps.v1.Deploymentj
x-kubernetes-actionput
jN
x-kubernetes-group-version-kind+)group: apps
version: v1
kind: Deployment
:…
apps_v1delete a Deployment* deleteAppsV1NamespacedDeployment2ù
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
2#/components/schemas/io.k8s.api.apps.v1.Deployment
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.apps.v1.Deployment
L
application/yaml8
64
2#/components/schemas/io.k8s.api.apps.v1.Deploymentï
202ç
ä
Accepted˝
L
application/json8
64
2#/components/schemas/io.k8s.api.apps.v1.Deployment
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.apps.v1.Deployment
L
application/yaml8
64
2#/components/schemas/io.k8s.api.apps.v1.Deploymentj 
x-kubernetes-action	delete
jN
x-kubernetes-group-version-kind+)group: apps
version: v1
kind: Deployment
R°
apps_v1)partially update the specified Deployment*patchAppsV1NamespacedDeployment2ù
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
2#/components/schemas/io.k8s.api.apps.v1.Deployment
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.apps.v1.Deployment
L
application/yaml8
64
2#/components/schemas/io.k8s.api.apps.v1.Deploymentî
201å
â
Created˝
L
application/json8
64
2#/components/schemas/io.k8s.api.apps.v1.Deployment
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.apps.v1.Deployment
L
application/yaml8
64
2#/components/schemas/io.k8s.api.apps.v1.Deploymentj
x-kubernetes-actionpatch
jN
x-kubernetes-group-version-kind+)group: apps
version: v1
kind: Deployment
j8
6
namepathname of the Deployment R
† stringja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
¶2
=/apis/apps/v1/namespaces/{namespace}/deployments/{name}/scale‰1"‚
apps_v1&read scale of the specified Deployment*#readAppsV1NamespacedDeploymentScaleBòï
200ç
ä
OKÉ
N
application/json:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scale
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scale
N
application/yaml:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scalej
x-kubernetes-actionget
jP
x-kubernetes-group-version-kind-+group: autoscaling
version: v1
kind: Scale
*Ê
apps_v1)replace scale of the specified Deployment*&replaceAppsV1NamespacedDeploymentScale2ù
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
4#/components/schemas/io.k8s.api.autoscaling.v1.ScaleBµï
200ç
ä
OKÉ
N
application/json:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scale
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scale
N
application/yaml:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scaleö
201í
è
CreatedÉ
N
application/json:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scale
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scale
N
application/yaml:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scalej
x-kubernetes-actionput
jP
x-kubernetes-group-version-kind-+group: autoscaling
version: v1
kind: Scale
RΩ
apps_v12partially update scale of the specified Deployment*$patchAppsV1NamespacedDeploymentScale2ù
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
4#/components/schemas/io.k8s.api.autoscaling.v1.Scale
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scale
N
application/yaml:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scaleö
201í
è
CreatedÉ
N
application/json:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scale
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scale
N
application/yaml:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scalej
x-kubernetes-actionpatch
jP
x-kubernetes-group-version-kind-+group: autoscaling
version: v1
kind: Scale
j3
1
namepathname of the Scale R
† stringja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
å2
>/apis/apps/v1/namespaces/{namespace}/deployments/{name}/status…1"‹
apps_v1'read status of the specified Deployment*$readAppsV1NamespacedDeploymentStatusBíè
200á
Ñ
OK˝
L
application/json8
64
2#/components/schemas/io.k8s.api.apps.v1.Deployment
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.apps.v1.Deployment
L
application/yaml8
64
2#/components/schemas/io.k8s.api.apps.v1.Deploymentj
x-kubernetes-actionget
jN
x-kubernetes-group-version-kind+)group: apps
version: v1
kind: Deployment
*ÿ
apps_v1*replace status of the specified Deployment*'replaceAppsV1NamespacedDeploymentStatus2ù
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
2#/components/schemas/io.k8s.api.apps.v1.DeploymentB©è
200á
Ñ
OK˝
L
application/json8
64
2#/components/schemas/io.k8s.api.apps.v1.Deployment
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.apps.v1.Deployment
L
application/yaml8
64
2#/components/schemas/io.k8s.api.apps.v1.Deploymentî
201å
â
Created˝
L
application/json8
64
2#/components/schemas/io.k8s.api.apps.v1.Deployment
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.apps.v1.Deployment
L
application/yaml8
64
2#/components/schemas/io.k8s.api.apps.v1.Deploymentj
x-kubernetes-actionput
jN
x-kubernetes-group-version-kind+)group: apps
version: v1
kind: Deployment
R±
apps_v13partially update status of the specified Deployment*%patchAppsV1NamespacedDeploymentStatus2ù
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
2#/components/schemas/io.k8s.api.apps.v1.Deployment
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.apps.v1.Deployment
L
application/yaml8
64
2#/components/schemas/io.k8s.api.apps.v1.Deploymentî
201å
â
Created˝
L
application/json8
64
2#/components/schemas/io.k8s.api.apps.v1.Deployment
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.apps.v1.Deployment
L
application/yaml8
64
2#/components/schemas/io.k8s.api.apps.v1.Deploymentj
x-kubernetes-actionpatch
jN
x-kubernetes-group-version-kind+)group: apps
version: v1
kind: Deployment
j8
6
namepathname of the Deployment R
† stringja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
’°
0/apis/apps/v1/namespaces/{namespace}/replicasetsü°"ﬁ>
apps_v1(list or watch objects of kind ReplicaSet*listAppsV1NamespacedReplicaSet2™
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
6#/components/schemas/io.k8s.api.apps.v1.ReplicaSetList
]
application/json;stream=watch<
:8
6#/components/schemas/io.k8s.api.apps.v1.ReplicaSetList
c
#application/vnd.kubernetes.protobuf<
:8
6#/components/schemas/io.k8s.api.apps.v1.ReplicaSetList
p
0application/vnd.kubernetes.protobuf;stream=watch<
:8
6#/components/schemas/io.k8s.api.apps.v1.ReplicaSetList
P
application/yaml<
:8
6#/components/schemas/io.k8s.api.apps.v1.ReplicaSetListj
x-kubernetes-actionlist
jN
x-kubernetes-group-version-kind+)group: apps
version: v1
kind: ReplicaSet
2”
apps_v1create a ReplicaSet* createAppsV1NamespacedReplicaSet2ù
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
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSetB¡è
200á
Ñ
OK˝
L
application/json8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSet
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSet
L
application/yaml8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSetî
201å
â
Created˝
L
application/json8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSet
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSet
L
application/yaml8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSetï
202ç
ä
Accepted˝
L
application/json8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSet
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSet
L
application/yaml8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSetj
x-kubernetes-actionpost
jN
x-kubernetes-group-version-kind+)group: apps
version: v1
kind: ReplicaSet
:ƒK
apps_v1delete collection of ReplicaSet**deleteAppsV1CollectionNamespacedReplicaSet2Ó	
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
jN
x-kubernetes-group-version-kind+)group: apps
version: v1
kind: ReplicaSet
ja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
°J
7/apis/apps/v1/namespaces/{namespace}/replicasets/{name}ÂI"Ã
apps_v1read the specified ReplicaSet*readAppsV1NamespacedReplicaSetBíè
200á
Ñ
OK˝
L
application/json8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSet
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSet
L
application/yaml8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSetj
x-kubernetes-actionget
jN
x-kubernetes-group-version-kind+)group: apps
version: v1
kind: ReplicaSet
*»
apps_v1 replace the specified ReplicaSet*!replaceAppsV1NamespacedReplicaSet2ù
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
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSetB©è
200á
Ñ
OK˝
L
application/json8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSet
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSet
L
application/yaml8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSetî
201å
â
Created˝
L
application/json8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSet
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSet
L
application/yaml8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSetj
x-kubernetes-actionput
jN
x-kubernetes-group-version-kind+)group: apps
version: v1
kind: ReplicaSet
:…
apps_v1delete a ReplicaSet* deleteAppsV1NamespacedReplicaSet2ù
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
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSet
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSet
L
application/yaml8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSetï
202ç
ä
Accepted˝
L
application/json8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSet
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSet
L
application/yaml8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSetj 
x-kubernetes-action	delete
jN
x-kubernetes-group-version-kind+)group: apps
version: v1
kind: ReplicaSet
R°
apps_v1)partially update the specified ReplicaSet*patchAppsV1NamespacedReplicaSet2ù
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
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSet
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSet
L
application/yaml8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSetî
201å
â
Created˝
L
application/json8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSet
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSet
L
application/yaml8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSetj
x-kubernetes-actionpatch
jN
x-kubernetes-group-version-kind+)group: apps
version: v1
kind: ReplicaSet
j8
6
namepathname of the ReplicaSet R
† stringja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
¶2
=/apis/apps/v1/namespaces/{namespace}/replicasets/{name}/scale‰1"‚
apps_v1&read scale of the specified ReplicaSet*#readAppsV1NamespacedReplicaSetScaleBòï
200ç
ä
OKÉ
N
application/json:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scale
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scale
N
application/yaml:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scalej
x-kubernetes-actionget
jP
x-kubernetes-group-version-kind-+group: autoscaling
version: v1
kind: Scale
*Ê
apps_v1)replace scale of the specified ReplicaSet*&replaceAppsV1NamespacedReplicaSetScale2ù
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
4#/components/schemas/io.k8s.api.autoscaling.v1.ScaleBµï
200ç
ä
OKÉ
N
application/json:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scale
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scale
N
application/yaml:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scaleö
201í
è
CreatedÉ
N
application/json:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scale
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scale
N
application/yaml:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scalej
x-kubernetes-actionput
jP
x-kubernetes-group-version-kind-+group: autoscaling
version: v1
kind: Scale
RΩ
apps_v12partially update scale of the specified ReplicaSet*$patchAppsV1NamespacedReplicaSetScale2ù
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
4#/components/schemas/io.k8s.api.autoscaling.v1.Scale
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scale
N
application/yaml:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scaleö
201í
è
CreatedÉ
N
application/json:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scale
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scale
N
application/yaml:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scalej
x-kubernetes-actionpatch
jP
x-kubernetes-group-version-kind-+group: autoscaling
version: v1
kind: Scale
j3
1
namepathname of the Scale R
† stringja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
å2
>/apis/apps/v1/namespaces/{namespace}/replicasets/{name}/status…1"‹
apps_v1'read status of the specified ReplicaSet*$readAppsV1NamespacedReplicaSetStatusBíè
200á
Ñ
OK˝
L
application/json8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSet
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSet
L
application/yaml8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSetj
x-kubernetes-actionget
jN
x-kubernetes-group-version-kind+)group: apps
version: v1
kind: ReplicaSet
*ÿ
apps_v1*replace status of the specified ReplicaSet*'replaceAppsV1NamespacedReplicaSetStatus2ù
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
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSetB©è
200á
Ñ
OK˝
L
application/json8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSet
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSet
L
application/yaml8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSetî
201å
â
Created˝
L
application/json8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSet
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSet
L
application/yaml8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSetj
x-kubernetes-actionput
jN
x-kubernetes-group-version-kind+)group: apps
version: v1
kind: ReplicaSet
R±
apps_v13partially update status of the specified ReplicaSet*%patchAppsV1NamespacedReplicaSetStatus2ù
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
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSet
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSet
L
application/yaml8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSetî
201å
â
Created˝
L
application/json8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSet
_
#application/vnd.kubernetes.protobuf8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSet
L
application/yaml8
64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSetj
x-kubernetes-actionpatch
jN
x-kubernetes-group-version-kind+)group: apps
version: v1
kind: ReplicaSet
j8
6
namepathname of the ReplicaSet R
† stringja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
Ó°
1/apis/apps/v1/namespaces/{namespace}/statefulsets∑°"Ê>
apps_v1)list or watch objects of kind StatefulSet*listAppsV1NamespacedStatefulSet2™
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
† booleanBÙÒ
200È
Ê
OKﬂ
Q
application/json=
;9
7#/components/schemas/io.k8s.api.apps.v1.StatefulSetList
^
application/json;stream=watch=
;9
7#/components/schemas/io.k8s.api.apps.v1.StatefulSetList
d
#application/vnd.kubernetes.protobuf=
;9
7#/components/schemas/io.k8s.api.apps.v1.StatefulSetList
q
0application/vnd.kubernetes.protobuf;stream=watch=
;9
7#/components/schemas/io.k8s.api.apps.v1.StatefulSetList
Q
application/yaml=
;9
7#/components/schemas/io.k8s.api.apps.v1.StatefulSetListj
x-kubernetes-actionlist
jO
x-kubernetes-group-version-kind,*group: apps
version: v1
kind: StatefulSet
2‡
apps_v1create a StatefulSet*!createAppsV1NamespacedStatefulSet2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:H
FB
@
*/*9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSetB í
200ä
á
OKÄ
M
application/json9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSet
`
#application/vnd.kubernetes.protobuf9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSet
M
application/yaml9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSetó
201è
å
CreatedÄ
M
application/json9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSet
`
#application/vnd.kubernetes.protobuf9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSet
M
application/yaml9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSetò
202ê
ç
AcceptedÄ
M
application/json9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSet
`
#application/vnd.kubernetes.protobuf9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSet
M
application/yaml9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSetj
x-kubernetes-actionpost
jO
x-kubernetes-group-version-kind,*group: apps
version: v1
kind: StatefulSet
:«K
apps_v1 delete collection of StatefulSet*+deleteAppsV1CollectionNamespacedStatefulSet2Ó	
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
jO
x-kubernetes-group-version-kind,*group: apps
version: v1
kind: StatefulSet
ja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
≈J
8/apis/apps/v1/namespaces/{namespace}/statefulsets/{name}àJ"“
apps_v1read the specified StatefulSet*readAppsV1NamespacedStatefulSetBïí
200ä
á
OKÄ
M
application/json9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSet
`
#application/vnd.kubernetes.protobuf9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSet
M
application/yaml9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSetj
x-kubernetes-actionget
jO
x-kubernetes-group-version-kind,*group: apps
version: v1
kind: StatefulSet
*“
apps_v1!replace the specified StatefulSet*"replaceAppsV1NamespacedStatefulSet2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:H
FB
@
*/*9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSetBØí
200ä
á
OKÄ
M
application/json9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSet
`
#application/vnd.kubernetes.protobuf9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSet
M
application/yaml9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSetó
201è
å
CreatedÄ
M
application/json9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSet
`
#application/vnd.kubernetes.protobuf9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSet
M
application/yaml9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSetj
x-kubernetes-actionput
jO
x-kubernetes-group-version-kind,*group: apps
version: v1
kind: StatefulSet
:“
apps_v1delete a StatefulSet*!deleteAppsV1NamespacedStatefulSet2ù
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
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptionsB∞í
200ä
á
OKÄ
M
application/json9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSet
`
#application/vnd.kubernetes.protobuf9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSet
M
application/yaml9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSetò
202ê
ç
AcceptedÄ
M
application/json9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSet
`
#application/vnd.kubernetes.protobuf9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSet
M
application/yaml9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSetj 
x-kubernetes-action	delete
jO
x-kubernetes-group-version-kind,*group: apps
version: v1
kind: StatefulSet
R™
apps_v1*partially update the specified StatefulSet* patchAppsV1NamespacedStatefulSet2ù
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
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchBØí
200ä
á
OKÄ
M
application/json9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSet
`
#application/vnd.kubernetes.protobuf9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSet
M
application/yaml9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSetó
201è
å
CreatedÄ
M
application/json9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSet
`
#application/vnd.kubernetes.protobuf9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSet
M
application/yaml9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSetj
x-kubernetes-actionpatch
jO
x-kubernetes-group-version-kind,*group: apps
version: v1
kind: StatefulSet
j9
7
namepathname of the StatefulSet R
† stringja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
≠2
>/apis/apps/v1/namespaces/{namespace}/statefulsets/{name}/scaleÍ1"‰
apps_v1'read scale of the specified StatefulSet*$readAppsV1NamespacedStatefulSetScaleBòï
200ç
ä
OKÉ
N
application/json:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scale
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scale
N
application/yaml:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scalej
x-kubernetes-actionget
jP
x-kubernetes-group-version-kind-+group: autoscaling
version: v1
kind: Scale
*Ë
apps_v1*replace scale of the specified StatefulSet*'replaceAppsV1NamespacedStatefulSetScale2ù
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
4#/components/schemas/io.k8s.api.autoscaling.v1.ScaleBµï
200ç
ä
OKÉ
N
application/json:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scale
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scale
N
application/yaml:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scaleö
201í
è
CreatedÉ
N
application/json:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scale
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scale
N
application/yaml:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scalej
x-kubernetes-actionput
jP
x-kubernetes-group-version-kind-+group: autoscaling
version: v1
kind: Scale
Rø
apps_v13partially update scale of the specified StatefulSet*%patchAppsV1NamespacedStatefulSetScale2ù
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
4#/components/schemas/io.k8s.api.autoscaling.v1.Scale
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scale
N
application/yaml:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scaleö
201í
è
CreatedÉ
N
application/json:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scale
a
#application/vnd.kubernetes.protobuf:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scale
N
application/yaml:
86
4#/components/schemas/io.k8s.api.autoscaling.v1.Scalej
x-kubernetes-actionpatch
jP
x-kubernetes-group-version-kind-+group: autoscaling
version: v1
kind: Scale
j3
1
namepathname of the Scale R
† stringja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
ß2
?/apis/apps/v1/namespaces/{namespace}/statefulsets/{name}/status„1"‚
apps_v1(read status of the specified StatefulSet*%readAppsV1NamespacedStatefulSetStatusBïí
200ä
á
OKÄ
M
application/json9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSet
`
#application/vnd.kubernetes.protobuf9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSet
M
application/yaml9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSetj
x-kubernetes-actionget
jO
x-kubernetes-group-version-kind,*group: apps
version: v1
kind: StatefulSet
*‚
apps_v1+replace status of the specified StatefulSet*(replaceAppsV1NamespacedStatefulSetStatus2ù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† string2ï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† string2€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† string:H
FB
@
*/*9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSetBØí
200ä
á
OKÄ
M
application/json9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSet
`
#application/vnd.kubernetes.protobuf9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSet
M
application/yaml9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSetó
201è
å
CreatedÄ
M
application/json9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSet
`
#application/vnd.kubernetes.protobuf9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSet
M
application/yaml9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSetj
x-kubernetes-actionput
jO
x-kubernetes-group-version-kind,*group: apps
version: v1
kind: StatefulSet
R∫
apps_v14partially update status of the specified StatefulSet*&patchAppsV1NamespacedStatefulSetStatus2ù
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
?#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.PatchBØí
200ä
á
OKÄ
M
application/json9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSet
`
#application/vnd.kubernetes.protobuf9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSet
M
application/yaml9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSetó
201è
å
CreatedÄ
M
application/json9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSet
`
#application/vnd.kubernetes.protobuf9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSet
M
application/yaml9
75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSetj
x-kubernetes-actionpatch
jO
x-kubernetes-group-version-kind,*group: apps
version: v1
kind: StatefulSet
j9
7
namepathname of the StatefulSet R
† stringja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
√@
/apis/apps/v1/replicasets•@"ª
apps_v1(list or watch objects of kind ReplicaSet*$listAppsV1ReplicaSetForAllNamespacesBÔÏ
200‰
·
OK⁄
P
application/json<
:8
6#/components/schemas/io.k8s.api.apps.v1.ReplicaSetList
]
application/json;stream=watch<
:8
6#/components/schemas/io.k8s.api.apps.v1.ReplicaSetList
c
#application/vnd.kubernetes.protobuf<
:8
6#/components/schemas/io.k8s.api.apps.v1.ReplicaSetList
p
0application/vnd.kubernetes.protobuf;stream=watch<
:8
6#/components/schemas/io.k8s.api.apps.v1.ReplicaSetList
P
application/yaml<
:8
6#/components/schemas/io.k8s.api.apps.v1.ReplicaSetListj
x-kubernetes-actionlist
jN
x-kubernetes-group-version-kind+)group: apps
version: v1
kind: ReplicaSet
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
Ã@
/apis/apps/v1/statefulsets≠@"√
apps_v1)list or watch objects of kind StatefulSet*%listAppsV1StatefulSetForAllNamespacesBÙÒ
200È
Ê
OKﬂ
Q
application/json=
;9
7#/components/schemas/io.k8s.api.apps.v1.StatefulSetList
^
application/json;stream=watch=
;9
7#/components/schemas/io.k8s.api.apps.v1.StatefulSetList
d
#application/vnd.kubernetes.protobuf=
;9
7#/components/schemas/io.k8s.api.apps.v1.StatefulSetList
q
0application/vnd.kubernetes.protobuf;stream=watch=
;9
7#/components/schemas/io.k8s.api.apps.v1.StatefulSetList
Q
application/yaml=
;9
7#/components/schemas/io.k8s.api.apps.v1.StatefulSetListj
x-kubernetes-actionlist
jO
x-kubernetes-group-version-kind,*group: apps
version: v1
kind: StatefulSet
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
áB
'/apis/apps/v1/watch/controllerrevisions€A"Ò
apps_v1~watch individual changes to a list of ControllerRevision. deprecated: use the 'watch' parameter with a list operation instead.*1watchAppsV1ControllerRevisionListForAllNamespacesBµ≤
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
jV
x-kubernetes-group-version-kind31group: apps
version: v1
kind: ControllerRevision
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
„A
/apis/apps/v1/watch/daemonsets¿A"÷
apps_v1uwatch individual changes to a list of DaemonSet. deprecated: use the 'watch' parameter with a list operation instead.*(watchAppsV1DaemonSetListForAllNamespacesBµ≤
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
jM
x-kubernetes-group-version-kind*(group: apps
version: v1
kind: DaemonSet
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
ÁA
/apis/apps/v1/watch/deployments√A"Ÿ
apps_v1vwatch individual changes to a list of Deployment. deprecated: use the 'watch' parameter with a list operation instead.*)watchAppsV1DeploymentListForAllNamespacesBµ≤
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
jN
x-kubernetes-group-version-kind+)group: apps
version: v1
kind: Deployment
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
˚B
>/apis/apps/v1/watch/namespaces/{namespace}/controllerrevisions∏B"Î
apps_v1~watch individual changes to a list of ControllerRevision. deprecated: use the 'watch' parameter with a list operation instead.*+watchAppsV1NamespacedControllerRevisionListBµ≤
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
jV
x-kubernetes-group-version-kind31group: apps
version: v1
kind: ControllerRevision
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
¯C
E/apis/apps/v1/watch/namespaces/{namespace}/controllerrevisions/{name}ÆC"ü
apps_v1πwatch changes to an object of kind ControllerRevision. deprecated: use the 'watch' parameter with a list operation instead, filtered to a single item with the 'fieldSelector' parameter.*'watchAppsV1NamespacedControllerRevisionBµ≤
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
jV
x-kubernetes-group-version-kind31group: apps
version: v1
kind: ControllerRevision
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
namepathname of the ControllerRevision R
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
◊B
5/apis/apps/v1/watch/namespaces/{namespace}/daemonsetsùB"–
apps_v1uwatch individual changes to a list of DaemonSet. deprecated: use the 'watch' parameter with a list operation instead.*"watchAppsV1NamespacedDaemonSetListBµ≤
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
jM
x-kubernetes-group-version-kind*(group: apps
version: v1
kind: DaemonSet
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
ÀC
</apis/apps/v1/watch/namespaces/{namespace}/daemonsets/{name}äC"Ñ
apps_v1∞watch changes to an object of kind DaemonSet. deprecated: use the 'watch' parameter with a list operation instead, filtered to a single item with the 'fieldSelector' parameter.*watchAppsV1NamespacedDaemonSetBµ≤
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
jM
x-kubernetes-group-version-kind*(group: apps
version: v1
kind: DaemonSet
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
namepathname of the DaemonSet R
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
€B
6/apis/apps/v1/watch/namespaces/{namespace}/deployments†B"”
apps_v1vwatch individual changes to a list of Deployment. deprecated: use the 'watch' parameter with a list operation instead.*#watchAppsV1NamespacedDeploymentListBµ≤
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
jN
x-kubernetes-group-version-kind+)group: apps
version: v1
kind: Deployment
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
–C
=/apis/apps/v1/watch/namespaces/{namespace}/deployments/{name}éC"á
apps_v1±watch changes to an object of kind Deployment. deprecated: use the 'watch' parameter with a list operation instead, filtered to a single item with the 'fieldSelector' parameter.*watchAppsV1NamespacedDeploymentBµ≤
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
jN
x-kubernetes-group-version-kind+)group: apps
version: v1
kind: Deployment
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
† integerj8
6
namepathname of the Deployment R
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
€B
6/apis/apps/v1/watch/namespaces/{namespace}/replicasets†B"”
apps_v1vwatch individual changes to a list of ReplicaSet. deprecated: use the 'watch' parameter with a list operation instead.*#watchAppsV1NamespacedReplicaSetListBµ≤
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
jN
x-kubernetes-group-version-kind+)group: apps
version: v1
kind: ReplicaSet
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
–C
=/apis/apps/v1/watch/namespaces/{namespace}/replicasets/{name}éC"á
apps_v1±watch changes to an object of kind ReplicaSet. deprecated: use the 'watch' parameter with a list operation instead, filtered to a single item with the 'fieldSelector' parameter.*watchAppsV1NamespacedReplicaSetBµ≤
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
jN
x-kubernetes-group-version-kind+)group: apps
version: v1
kind: ReplicaSet
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
† integerj8
6
namepathname of the ReplicaSet R
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
ﬂB
7/apis/apps/v1/watch/namespaces/{namespace}/statefulsets£B"÷
apps_v1wwatch individual changes to a list of StatefulSet. deprecated: use the 'watch' parameter with a list operation instead.*$watchAppsV1NamespacedStatefulSetListBµ≤
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
jO
x-kubernetes-group-version-kind,*group: apps
version: v1
kind: StatefulSet
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
’C
>/apis/apps/v1/watch/namespaces/{namespace}/statefulsets/{name}íC"ä
apps_v1≤watch changes to an object of kind StatefulSet. deprecated: use the 'watch' parameter with a list operation instead, filtered to a single item with the 'fieldSelector' parameter.* watchAppsV1NamespacedStatefulSetBµ≤
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
jO
x-kubernetes-group-version-kind,*group: apps
version: v1
kind: StatefulSet
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
† integerj9
7
namepathname of the StatefulSet R
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
ÁA
/apis/apps/v1/watch/replicasets√A"Ÿ
apps_v1vwatch individual changes to a list of ReplicaSet. deprecated: use the 'watch' parameter with a list operation instead.*)watchAppsV1ReplicaSetListForAllNamespacesBµ≤
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
jN
x-kubernetes-group-version-kind+)group: apps
version: v1
kind: ReplicaSet
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
ÎA
 /apis/apps/v1/watch/statefulsets∆A"‹
apps_v1wwatch individual changes to a list of StatefulSet. deprecated: use the 'watch' parameter with a list operation instead.**watchAppsV1StatefulSetListForAllNamespacesBµ≤
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
jO
x-kubernetes-group-version-kind,*group: apps
version: v1
kind: StatefulSet
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
† boolean*˘‘
ı‘
Ø
%io.k8s.api.apps.v1.ControllerRevisionÖ
Ç∫revision object˙ã


apiVersion
	 string
M
dataEC
A#/components/schemas/io.k8s.apimachinery.pkg.runtime.RawExtension

kind
	 string
\
metadataP
N“HF
D#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMetaä 
,
revision 
 integerä		        öint64¢\
x-kubernetes-group-version-kind97- group: apps
  kind: ControllerRevision
  version: v1

ï
)io.k8s.api.apps.v1.ControllerRevisionListÁ
‰∫items object˙Ï


apiVersion
	 string
^
itemsU
S arrayÚH
F
D“><
:#/components/schemas/io.k8s.api.apps.v1.ControllerRevisionä 

kind
	 string
Z
metadataN
L“FD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaä ¢`
x-kubernetes-group-version-kind=;- group: apps
  kind: ControllerRevisionList
  version: v1

Ø
io.k8s.api.apps.v1.DaemonSeté
ã object˙®
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
I
specA
?“97
5#/components/schemas/io.k8s.api.apps.v1.DaemonSetSpecä 
M
statusC
A“;9
7#/components/schemas/io.k8s.api.apps.v1.DaemonSetStatusä ¢S
x-kubernetes-group-version-kind0.- group: apps
  kind: DaemonSet
  version: v1

â
%io.k8s.api.apps.v1.DaemonSetConditionﬂ
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
˙
 io.k8s.api.apps.v1.DaemonSetList’
“∫items object˙„


apiVersion
	 string
U
itemsL
J arrayÚ?
=
;“53
1#/components/schemas/io.k8s.api.apps.v1.DaemonSetä 

kind
	 string
Z
metadataN
L“FD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaä ¢W
x-kubernetes-group-version-kind42- group: apps
  kind: DaemonSetList
  version: v1

´
 io.k8s.api.apps.v1.DaemonSetSpecÜ
É∫selector∫template object˙‡
'
minReadySeconds
 integeröint32
,
revisionHistoryLimit
 integeröint32
W
selectorKI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.LabelSelector
O
templateC
A“;9
7#/components/schemas/io.k8s.api.core.v1.PodTemplateSpecä 
]
updateStrategyK
I“CA
?#/components/schemas/io.k8s.api.apps.v1.DaemonSetUpdateStrategyä 
Œ
"io.k8s.api.apps.v1.DaemonSetStatusß
§∫currentNumberScheduled∫numberMisscheduled∫desiredNumberScheduled∫numberReady object˙¬
&
collisionCount
 integeröint32
Ü

conditions˜
Ù arrayÚH
F
D“><
:#/components/schemas/io.k8s.api.apps.v1.DaemonSetConditionä ¢'
x-kubernetes-list-map-keys	- type
¢ 
x-kubernetes-list-typemap
¢'
x-kubernetes-patch-merge-keytype
¢'
x-kubernetes-patch-strategymerge

:
currentNumberScheduled 
 integerä		        öint32
:
desiredNumberScheduled 
 integerä		        öint32
'
numberAvailable
 integeröint32
6
numberMisscheduled 
 integerä		        öint32
/
numberReady 
 integerä		        öint32
)
numberUnavailable
 integeröint32
*
observedGeneration
 integeröint64
.
updatedNumberScheduled
 integeröint32
 
*io.k8s.api.apps.v1.DaemonSetUpdateStrategyõ
ò object˙ã
S
rollingUpdateB@
>#/components/schemas/io.k8s.api.apps.v1.RollingUpdateDaemonSet
4
type,
*¬	OnDelete
¬RollingUpdate
 string
≥
io.k8s.api.apps.v1.Deploymentë
é object˙™
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
6#/components/schemas/io.k8s.api.apps.v1.DeploymentSpecä 
N
statusD
B“<:
8#/components/schemas/io.k8s.api.apps.v1.DeploymentStatusä ¢T
x-kubernetes-group-version-kind1/- group: apps
  kind: Deployment
  version: v1

‡
&io.k8s.api.apps.v1.DeploymentConditionµ
≤∫type∫status object˙ï
X
lastTransitionTimeB@
>#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.Time
T
lastUpdateTimeB@
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
˝
!io.k8s.api.apps.v1.DeploymentList◊
‘∫items object˙‰


apiVersion
	 string
V
itemsM
K arrayÚ@
>
<“64
2#/components/schemas/io.k8s.api.apps.v1.Deploymentä 

kind
	 string
Z
metadataN
L“FD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaä ¢X
x-kubernetes-group-version-kind53- group: apps
  kind: DeploymentList
  version: v1

º
!io.k8s.api.apps.v1.DeploymentSpecñ
ì∫selector∫template object˙
'
minReadySeconds
 integeröint32

paused

 boolean
/
progressDeadlineSeconds
 integeröint32
 
replicas
 integeröint32
,
revisionHistoryLimit
 integeröint32
W
selectorKI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.LabelSelector
Å
strategyu
s“><
:#/components/schemas/io.k8s.api.apps.v1.DeploymentStrategyä ¢,
x-kubernetes-patch-strategyretainKeys

O
templateC
A“;9
7#/components/schemas/io.k8s.api.core.v1.PodTemplateSpecä 
ç
#io.k8s.api.apps.v1.DeploymentStatusÂ
‚ object˙’
)
availableReplicas
 integeröint32
&
collisionCount
 integeröint32
á

conditions¯
ı arrayÚI
G
E“?=
;#/components/schemas/io.k8s.api.apps.v1.DeploymentConditionä ¢'
x-kubernetes-list-map-keys	- type
¢ 
x-kubernetes-list-typemap
¢'
x-kubernetes-patch-merge-keytype
¢'
x-kubernetes-patch-strategymerge

*
observedGeneration
 integeröint64
%
readyReplicas
 integeröint32
 
replicas
 integeröint32
+
terminatingReplicas
 integeröint32
+
unavailableReplicas
 integeröint32
'
updatedReplicas
 integeröint32
∆
%io.k8s.api.apps.v1.DeploymentStrategyú
ô object˙å
T
rollingUpdateCA
?#/components/schemas/io.k8s.api.apps.v1.RollingUpdateDeployment
4
type,
*¬	Recreate
¬RollingUpdate
 string
≥
io.k8s.api.apps.v1.ReplicaSetë
é object˙™
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
6#/components/schemas/io.k8s.api.apps.v1.ReplicaSetSpecä 
N
statusD
B“<:
8#/components/schemas/io.k8s.api.apps.v1.ReplicaSetStatusä ¢T
x-kubernetes-group-version-kind1/- group: apps
  kind: ReplicaSet
  version: v1

ä
&io.k8s.api.apps.v1.ReplicaSetConditionﬂ
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
˝
!io.k8s.api.apps.v1.ReplicaSetList◊
‘∫items object˙‰


apiVersion
	 string
V
itemsM
K arrayÚ@
>
<“64
2#/components/schemas/io.k8s.api.apps.v1.ReplicaSetä 

kind
	 string
Z
metadataN
L“FD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaä ¢X
x-kubernetes-group-version-kind53- group: apps
  kind: ReplicaSetList
  version: v1

∂
!io.k8s.api.apps.v1.ReplicaSetSpecê
ç∫selector object˙ı
'
minReadySeconds
 integeröint32
 
replicas
 integeröint32
W
selectorKI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.LabelSelector
O
templateC
A“;9
7#/components/schemas/io.k8s.api.core.v1.PodTemplateSpecä 
‘
#io.k8s.api.apps.v1.ReplicaSetStatus¨
©∫replicas object˙ë
)
availableReplicas
 integeröint32
á

conditions¯
ı arrayÚI
G
E“?=
;#/components/schemas/io.k8s.api.apps.v1.ReplicaSetConditionä ¢'
x-kubernetes-list-map-keys	- type
¢ 
x-kubernetes-list-typemap
¢'
x-kubernetes-patch-merge-keytype
¢'
x-kubernetes-patch-strategymerge

,
fullyLabeledReplicas
 integeröint32
*
observedGeneration
 integeröint64
%
readyReplicas
 integeröint32
,
replicas 
 integerä		        öint32
+
terminatingReplicas
 integeröint32

)io.k8s.api.apps.v1.RollingUpdateDaemonSet¬
ø object˙≤
T
maxSurgeHF
D#/components/schemas/io.k8s.apimachinery.pkg.util.intstr.IntOrString
Z
maxUnavailableHF
D#/components/schemas/io.k8s.apimachinery.pkg.util.intstr.IntOrString
Ò
*io.k8s.api.apps.v1.RollingUpdateDeployment¬
ø object˙≤
T
maxSurgeHF
D#/components/schemas/io.k8s.apimachinery.pkg.util.intstr.IntOrString
Z
maxUnavailableHF
D#/components/schemas/io.k8s.apimachinery.pkg.util.intstr.IntOrString
∆
3io.k8s.api.apps.v1.RollingUpdateStatefulSetStrategyé
ã object˙
Z
maxUnavailableHF
D#/components/schemas/io.k8s.apimachinery.pkg.util.intstr.IntOrString
!
	partition
 integeröint32
∑
io.k8s.api.apps.v1.StatefulSetî
ë object˙¨
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
K
specC
A“;9
7#/components/schemas/io.k8s.api.apps.v1.StatefulSetSpecä 
O
statusE
C“=;
9#/components/schemas/io.k8s.api.apps.v1.StatefulSetStatusä ¢U
x-kubernetes-group-version-kind20- group: apps
  kind: StatefulSet
  version: v1

ã
'io.k8s.api.apps.v1.StatefulSetConditionﬂ
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
Ä
"io.k8s.api.apps.v1.StatefulSetListŸ
÷∫items object˙Â


apiVersion
	 string
W
itemsN
L arrayÚA
?
=“75
3#/components/schemas/io.k8s.api.apps.v1.StatefulSetä 

kind
	 string
Z
metadataN
L“FD
B#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ListMetaä ¢Y
x-kubernetes-group-version-kind64- group: apps
  kind: StatefulSetList
  version: v1

c
&io.k8s.api.apps.v1.StatefulSetOrdinals9
7 object˙+
)
start 
 integerä		        öint32
ã
Bio.k8s.api.apps.v1.StatefulSetPersistentVolumeClaimRetentionPolicyE
C object˙7

whenDeleted
	 string


whenScaled
	 string
¢
"io.k8s.api.apps.v1.StatefulSetSpec˚
¯∫selector∫template object˙’
'
minReadySeconds
 integeröint32
K
ordinals?=
;#/components/schemas/io.k8s.api.apps.v1.StatefulSetOrdinals
É
$persistentVolumeClaimRetentionPolicy[Y
W#/components/schemas/io.k8s.api.apps.v1.StatefulSetPersistentVolumeClaimRetentionPolicy
B
podManagementPolicy+
)¬OrderedReady
¬	Parallel
 string
 
replicas
 integeröint32
,
revisionHistoryLimit
 integeröint32
W
selectorKI
G#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.LabelSelector

serviceName
 stringä 
O
templateC
A“;9
7#/components/schemas/io.k8s.api.core.v1.PodTemplateSpecä 
_
updateStrategyM
K“EC
A#/components/schemas/io.k8s.api.apps.v1.StatefulSetUpdateStrategyä 
ñ
volumeClaimTemplates~
| arrayÚK
I
G“A?
=#/components/schemas/io.k8s.api.core.v1.PersistentVolumeClaimä ¢#
x-kubernetes-list-type	atomic

¿
$io.k8s.api.apps.v1.StatefulSetStatusó
î∫replicas object˙¸
5
availableReplicas 
 integerä		        öint32
&
collisionCount
 integeröint32
à

conditions˘
ˆ arrayÚJ
H
F“@>
<#/components/schemas/io.k8s.api.apps.v1.StatefulSetConditionä ¢'
x-kubernetes-list-map-keys	- type
¢ 
x-kubernetes-list-typemap
¢'
x-kubernetes-patch-merge-keytype
¢'
x-kubernetes-patch-strategymerge

'
currentReplicas
 integeröint32

currentRevision
	 string
*
observedGeneration
 integeröint64
%
readyReplicas
 integeröint32
,
replicas 
 integerä		        öint32

updateRevision
	 string
'
updatedReplicas
 integeröint32
÷
,io.k8s.api.apps.v1.StatefulSetUpdateStrategy•
¢ object˙ï
]
rollingUpdateLJ
H#/components/schemas/io.k8s.api.apps.v1.RollingUpdateStatefulSetStrategy
4
type,
*¬	OnDelete
¬RollingUpdate
 string
ê
io.k8s.api.autoscaling.v1.ScaleÏ
È object˙Æ
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
8#/components/schemas/io.k8s.api.autoscaling.v1.ScaleSpecä 
P
statusF
D“><
:#/components/schemas/io.k8s.api.autoscaling.v1.ScaleStatusä ¢™
x-kubernetes-group-version-kindÜÉ- group: ""
  kind: Scale
  version: v1
- group: apps
  kind: Scale
  version: v1
- group: autoscaling
  kind: Scale
  version: v1

c
#io.k8s.api.autoscaling.v1.ScaleSpec<
: object˙.
,
replicas 
 integerä		        öint32
â
%io.k8s.api.autoscaling.v1.ScaleStatus`
^∫replicas object˙G
,
replicas 
 integerä		        öint32

selector
	 string
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

ª
%io.k8s.api.core.v1.ModifyVolumeStatusë
é∫status object˙y
G
status=
;¬InProgress
¬Infeasible
¬
Pending
 stringä 
.
targetVolumeAttributesClassName
	 string
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

›
(io.k8s.api.core.v1.PersistentVolumeClaim∞
≠ object˙¿
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
specM
K“EC
A#/components/schemas/io.k8s.api.core.v1.PersistentVolumeClaimSpecä 
Y
statusO
M“GE
C#/components/schemas/io.k8s.api.core.v1.PersistentVolumeClaimStatusä ¢]
x-kubernetes-group-version-kind:8- group: ""
  kind: PersistentVolumeClaim
  version: v1

Í
1io.k8s.api.core.v1.PersistentVolumeClaimCondition¥
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
á	
.io.k8s.api.core.v1.PersistentVolumeClaimStatus‘
— object˙ƒ
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

Ù
allocatedResourceStatuses÷
” objectÇü
ú
ô¬ControllerResizeInProgress
¬ControllerResizeInfeasible
¬NodeResizeInProgress
¬NodeResizeInfeasible
¬NodeResizePending
 stringä ¢$
x-kubernetes-map-type	granular

l
allocatedResourcesV
T objectÇH
FD
B#/components/schemas/io.k8s.apimachinery.pkg.api.resource.Quantity
b
capacityV
T objectÇH
FD
B#/components/schemas/io.k8s.apimachinery.pkg.api.resource.Quantity
í

conditionsÉ
Ä arrayÚT
R
P“JH
F#/components/schemas/io.k8s.api.core.v1.PersistentVolumeClaimConditionä ¢'
x-kubernetes-list-map-keys	- type
¢ 
x-kubernetes-list-typemap
¢'
x-kubernetes-patch-merge-keytype
¢'
x-kubernetes-patch-strategymerge

/
 currentVolumeAttributesClassName
	 string
T
modifyVolumeStatus><
:#/components/schemas/io.k8s.api.core.v1.ModifyVolumeStatus
6
phase-
+¬Bound
¬Lost
¬
Pending
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

„
2io.k8s.apimachinery.pkg.apis.meta.v1.DeleteOptions¨
© object˙ë
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
	 string¢á
x-kubernetes-group-version-kinddb- group: ""
  kind: DeleteOptions
  version: v1
- group: apps
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
√
/io.k8s.apimachinery.pkg.apis.meta.v1.WatchEventè
å∫type∫object object˙k
O
objectEC
A#/components/schemas/io.k8s.apimachinery.pkg.runtime.RawExtension

type
 stringä ¢Å
x-kubernetes-group-version-kind^\- group: ""
  kind: WatchEvent
  version: v1
- group: apps
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