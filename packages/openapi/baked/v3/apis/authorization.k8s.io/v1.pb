
3.0.0

Kubernetes2v1.36.4+k8flare"Œg
À
/apis/authorization.k8s.io/v1/®"•
authorization_v1get available resources*getAuthorizationV1APIResourcesB◊‘
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
¸
N/apis/authorization.k8s.io/v1/namespaces/{namespace}/localsubjectaccessreviews©2Ô

authorization_v1!create a LocalSubjectAccessReview*7createAuthorizationV1NamespacedLocalSubjectAccessReview:^
\X
V
*/*O
MK
I#/components/schemas/io.k8s.api.authorization.v1.LocalSubjectAccessReviewBê‘
200Ã
…
OK¬
c
application/jsonO
MK
I#/components/schemas/io.k8s.api.authorization.v1.LocalSubjectAccessReview
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.authorization.v1.LocalSubjectAccessReview
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.authorization.v1.LocalSubjectAccessReviewŸ
201—
Œ
Created¬
c
application/jsonO
MK
I#/components/schemas/io.k8s.api.authorization.v1.LocalSubjectAccessReview
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.authorization.v1.LocalSubjectAccessReview
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.authorization.v1.LocalSubjectAccessReview⁄
202“
œ
Accepted¬
c
application/jsonO
MK
I#/components/schemas/io.k8s.api.authorization.v1.LocalSubjectAccessReview
v
#application/vnd.kubernetes.protobufO
MK
I#/components/schemas/io.k8s.api.authorization.v1.LocalSubjectAccessReview
c
application/yamlO
MK
I#/components/schemas/io.k8s.api.authorization.v1.LocalSubjectAccessReviewj
x-kubernetes-actionpost
jl
x-kubernetes-group-version-kindIGgroup: authorization.k8s.io
version: v1
kind: LocalSubjectAccessReview
jù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† stringjï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† stringj€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† stringja
_
	namespacepath:object name and auth scope, such as for teams and projects R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
Í
6/apis/authorization.k8s.io/v1/selfsubjectaccessreviewsØ2ÿ

authorization_v1 create a SelfSubjectAccessReview*,createAuthorizationV1SelfSubjectAccessReview:]
[W
U
*/*N
LJ
H#/components/schemas/io.k8s.api.authorization.v1.SelfSubjectAccessReviewBá—
200…
∆
OKø
b
application/jsonN
LJ
H#/components/schemas/io.k8s.api.authorization.v1.SelfSubjectAccessReview
u
#application/vnd.kubernetes.protobufN
LJ
H#/components/schemas/io.k8s.api.authorization.v1.SelfSubjectAccessReview
b
application/yamlN
LJ
H#/components/schemas/io.k8s.api.authorization.v1.SelfSubjectAccessReview÷
201Œ
À
Createdø
b
application/jsonN
LJ
H#/components/schemas/io.k8s.api.authorization.v1.SelfSubjectAccessReview
u
#application/vnd.kubernetes.protobufN
LJ
H#/components/schemas/io.k8s.api.authorization.v1.SelfSubjectAccessReview
b
application/yamlN
LJ
H#/components/schemas/io.k8s.api.authorization.v1.SelfSubjectAccessReview◊
202œ
Ã
Acceptedø
b
application/jsonN
LJ
H#/components/schemas/io.k8s.api.authorization.v1.SelfSubjectAccessReview
u
#application/vnd.kubernetes.protobufN
LJ
H#/components/schemas/io.k8s.api.authorization.v1.SelfSubjectAccessReview
b
application/yamlN
LJ
H#/components/schemas/io.k8s.api.authorization.v1.SelfSubjectAccessReviewj
x-kubernetes-actionpost
jk
x-kubernetes-group-version-kindHFgroup: authorization.k8s.io
version: v1
kind: SelfSubjectAccessReview
jù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† stringjï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† stringj€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
‹
5/apis/authorization.k8s.io/v1/selfsubjectrulesreviews¢2À

authorization_v1create a SelfSubjectRulesReview*+createAuthorizationV1SelfSubjectRulesReview:\
ZV
T
*/*M
KI
G#/components/schemas/io.k8s.api.authorization.v1.SelfSubjectRulesReviewB˛Œ
200∆
√
OKº
a
application/jsonM
KI
G#/components/schemas/io.k8s.api.authorization.v1.SelfSubjectRulesReview
t
#application/vnd.kubernetes.protobufM
KI
G#/components/schemas/io.k8s.api.authorization.v1.SelfSubjectRulesReview
a
application/yamlM
KI
G#/components/schemas/io.k8s.api.authorization.v1.SelfSubjectRulesReview”
201À
»
Createdº
a
application/jsonM
KI
G#/components/schemas/io.k8s.api.authorization.v1.SelfSubjectRulesReview
t
#application/vnd.kubernetes.protobufM
KI
G#/components/schemas/io.k8s.api.authorization.v1.SelfSubjectRulesReview
a
application/yamlM
KI
G#/components/schemas/io.k8s.api.authorization.v1.SelfSubjectRulesReview‘
202Ã
…
Acceptedº
a
application/jsonM
KI
G#/components/schemas/io.k8s.api.authorization.v1.SelfSubjectRulesReview
t
#application/vnd.kubernetes.protobufM
KI
G#/components/schemas/io.k8s.api.authorization.v1.SelfSubjectRulesReview
a
application/yamlM
KI
G#/components/schemas/io.k8s.api.authorization.v1.SelfSubjectRulesReviewj
x-kubernetes-actionpost
jj
x-kubernetes-group-version-kindGEgroup: authorization.k8s.io
version: v1
kind: SelfSubjectRulesReview
jù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† stringjï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† stringj€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string
≤
2/apis/authorization.k8s.io/v1/subjectaccessreviews˚2§

authorization_v1create a SubjectAccessReview*(createAuthorizationV1SubjectAccessReview:Y
WS
Q
*/*J
HF
D#/components/schemas/io.k8s.api.authorization.v1.SubjectAccessReviewB„≈
200Ω
∫
OK≥
^
application/jsonJ
HF
D#/components/schemas/io.k8s.api.authorization.v1.SubjectAccessReview
q
#application/vnd.kubernetes.protobufJ
HF
D#/components/schemas/io.k8s.api.authorization.v1.SubjectAccessReview
^
application/yamlJ
HF
D#/components/schemas/io.k8s.api.authorization.v1.SubjectAccessReview 
201¬
ø
Created≥
^
application/jsonJ
HF
D#/components/schemas/io.k8s.api.authorization.v1.SubjectAccessReview
q
#application/vnd.kubernetes.protobufJ
HF
D#/components/schemas/io.k8s.api.authorization.v1.SubjectAccessReview
^
application/yamlJ
HF
D#/components/schemas/io.k8s.api.authorization.v1.SubjectAccessReviewÀ
202√
¿
Accepted≥
^
application/jsonJ
HF
D#/components/schemas/io.k8s.api.authorization.v1.SubjectAccessReview
q
#application/vnd.kubernetes.protobufJ
HF
D#/components/schemas/io.k8s.api.authorization.v1.SubjectAccessReview
^
application/yamlJ
HF
D#/components/schemas/io.k8s.api.authorization.v1.SubjectAccessReviewj
x-kubernetes-actionpost
jg
x-kubernetes-group-version-kindDBgroup: authorization.k8s.io
version: v1
kind: SubjectAccessReview
jù
ö
dryRunquery¯When present, indicates that modifications should not be persisted. An invalid or unrecognized dryRun directive will result in an error response and no further processing of the request. Valid values are: - All: all dry run stages will be processedR
† stringjï
í
fieldManagerqueryÍfieldManager is a name associated with the actor or entity that is making these changes. The value must be less than or 128 characters long, and only contain printable characters, as defined by https://golang.org/pkg/unicode/#IsPrint.R
† stringj€
ÿ
fieldValidationquery≠fieldValidation instructs the server on how to handle objects in the request (POST/PUT/PATCH) containing unknown or duplicate fields. Valid values are: - Ignore: This will ignore any unknown fields that are silently dropped from the object, and will ignore all but the last duplicate field that the decoder encounters. This is the default behavior prior to v1.23. - Warn: This will send a warning via the standard warning response header for each unknown field that is dropped from the object, and for each duplicate field that is encountered. The request will still succeed if there are no other errors, and will only persist the last of any duplicate fields. This is the default in v1.23+ - Strict: This will fail the request with a BadRequest error if any unknown fields would be dropped from the object, or if any duplicate fields are present. The error returned from the server will contain all unknown and duplicate fields encountered.R
† stringjª
∏
prettyqueryñIf 'true', then the output is pretty printed. Defaults to 'false' unless the user-agent indicates a browser or command-line HTTP tool (curl and wget).R
† string*ÕD
 D
å
3io.k8s.api.authorization.v1.FieldSelectorAttributes‘
— object˙ƒ

rawSelector
	 string
•
requirementsî
ë arrayÚ`
^
\“VT
R#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.FieldSelectorRequirementä ¢#
x-kubernetes-list-type	atomic

å
3io.k8s.api.authorization.v1.LabelSelectorAttributes‘
— object˙ƒ

rawSelector
	 string
•
requirementsî
ë arrayÚ`
^
\“VT
R#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.LabelSelectorRequirementä ¢#
x-kubernetes-list-type	atomic

ì
4io.k8s.api.authorization.v1.LocalSubjectAccessReview⁄
◊∫spec object˙Œ
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
\
specT
R“LJ
H#/components/schemas/io.k8s.api.authorization.v1.SubjectAccessReviewSpecä 
`
statusV
T“NL
J#/components/schemas/io.k8s.api.authorization.v1.SubjectAccessReviewStatusä ¢r
x-kubernetes-group-version-kindOM- group: authorization.k8s.io
  kind: LocalSubjectAccessReview
  version: v1

m
1io.k8s.api.authorization.v1.NonResourceAttributes8
6 object˙*

path
	 string

verb
	 string
Ú
+io.k8s.api.authorization.v1.NonResourceRule¬
ø∫verbs object˙™
X
nonResourceURLsE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

N
verbsE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

®
.io.k8s.api.authorization.v1.ResourceAttributesı
Ú object˙Â
]
fieldSelectorLJ
H#/components/schemas/io.k8s.api.authorization.v1.FieldSelectorAttributes

group
	 string
]
labelSelectorLJ
H#/components/schemas/io.k8s.api.authorization.v1.LabelSelectorAttributes

name
	 string

	namespace
	 string

resource
	 string

subresource
	 string

verb
	 string

version
	 string
ï
(io.k8s.api.authorization.v1.ResourceRuleË
Â∫verbs object˙–
R
	apiGroupsE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

V
resourceNamesE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

R
	resourcesE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

N
verbsE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

ï
3io.k8s.api.authorization.v1.SelfSubjectAccessReview›
⁄∫spec object˙“
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
`
specX
V“PN
L#/components/schemas/io.k8s.api.authorization.v1.SelfSubjectAccessReviewSpecä 
`
statusV
T“NL
J#/components/schemas/io.k8s.api.authorization.v1.SubjectAccessReviewStatusä ¢q
x-kubernetes-group-version-kindNL- group: authorization.k8s.io
  kind: SelfSubjectAccessReview
  version: v1

ê
7io.k8s.api.authorization.v1.SelfSubjectAccessReviewSpec‘
— object˙ƒ
c
nonResourceAttributesJH
F#/components/schemas/io.k8s.api.authorization.v1.NonResourceAttributes
]
resourceAttributesGE
C#/components/schemas/io.k8s.api.authorization.v1.ResourceAttributes
ë
2io.k8s.api.authorization.v1.SelfSubjectRulesReview⁄
◊∫spec object˙–
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
_
specW
U“OM
K#/components/schemas/io.k8s.api.authorization.v1.SelfSubjectRulesReviewSpecä 
_
statusU
S“MK
I#/components/schemas/io.k8s.api.authorization.v1.SubjectRulesReviewStatusä ¢p
x-kubernetes-group-version-kindMK- group: authorization.k8s.io
  kind: SelfSubjectRulesReview
  version: v1

b
6io.k8s.api.authorization.v1.SelfSubjectRulesReviewSpec(
& object˙

	namespace
	 string
â
/io.k8s.api.authorization.v1.SubjectAccessReview’
“∫spec object˙Œ
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
\
specT
R“LJ
H#/components/schemas/io.k8s.api.authorization.v1.SubjectAccessReviewSpecä 
`
statusV
T“NL
J#/components/schemas/io.k8s.api.authorization.v1.SubjectAccessReviewStatusä ¢m
x-kubernetes-group-version-kindJH- group: authorization.k8s.io
  kind: SubjectAccessReview
  version: v1

¿
3io.k8s.api.authorization.v1.SubjectAccessReviewSpecà
Ö object˙¯
8
extra/
- objectÇ!

 arrayÚ

 stringä 
O
groupsE
C arrayÚ

 stringä ¢#
x-kubernetes-list-type	atomic

c
nonResourceAttributesJH
F#/components/schemas/io.k8s.api.authorization.v1.NonResourceAttributes
]
resourceAttributesGE
C#/components/schemas/io.k8s.api.authorization.v1.ResourceAttributes

uid
	 string

user
	 string
¿
5io.k8s.api.authorization.v1.SubjectAccessReviewStatusÜ
É∫allowed object˙m

allowed
 booleanä 

denied

 boolean

evaluationError
	 string

reason
	 string
Â
4io.k8s.api.authorization.v1.SubjectRulesReviewStatus¨
©∫resourceRules∫nonResourceRules∫
incomplete object˙Ï

evaluationError
	 string


incomplete
 booleanä 
ñ
nonResourceRulesÅ
 arrayÚN
L
J“DB
@#/components/schemas/io.k8s.api.authorization.v1.NonResourceRuleä ¢#
x-kubernetes-list-type	atomic

è
resourceRules~
| arrayÚK
I
G“A?
=#/components/schemas/io.k8s.api.authorization.v1.ResourceRuleä ¢#
x-kubernetes-list-type	atomic

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

Î
=io.k8s.apimachinery.pkg.apis.meta.v1.FieldSelectorRequirement©
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

<
-io.k8s.apimachinery.pkg.apis.meta.v1.FieldsV1
	 object
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

D
)io.k8s.apimachinery.pkg.apis.meta.v1.Time
 stringö	date-time