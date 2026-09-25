package jsonly

import (
	"k8s.io/apimachinery/pkg/runtime"
)

// JSONOnly hides the protobuf serializer from content negotiation. The js build
// strips the generated protobuf codecs from every API type (see the mirror's
// stripPB op), so a client that offered protobuf would otherwise be handed an
// encoder the types no longer implement and get a 500 instead of negotiating
// down to JSON.
type JSONOnly struct {
	runtime.NegotiatedSerializer
}

func (j JSONOnly) SupportedMediaTypes() []runtime.SerializerInfo {
	all := j.NegotiatedSerializer.SupportedMediaTypes()
	out := make([]runtime.SerializerInfo, 0, len(all))
	for _, info := range all {
		if info.MediaType == runtime.ContentTypeProtobuf {
			continue
		}
		out = append(out, info)
	}
	return out
}
