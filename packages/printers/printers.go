// Package printers serves upstream's table printers for one API group as a
// Worker Loader dynamic worker: POST /table with an external object or
// list returns the metav1.Table (columns and cells) upstream's printers
// produce for it. Row objects are left to the caller, which has them.
package printers

import (
	"encoding/json"
	"io"
	"net/http"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/api/legacyscheme"
	upstream "k8s.io/kubernetes/pkg/printers"
	printerstorage "k8s.io/kubernetes/pkg/printers/storage"
)

func Handler(addHandlers func(upstream.PrintHandler)) http.Handler {
	convertor := printerstorage.TableConvertor{TableGenerator: upstream.NewTableGenerator().With(addHandlers)}
	decoder := legacyscheme.Codecs.UniversalDecoder()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		obj, err := runtime.Decode(decoder, body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		table, err := convertor.ConvertToTable(r.Context(), obj, &metav1.TableOptions{})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for i := range table.Rows {
			table.Rows[i].Object = runtime.RawExtension{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(table)
	})
}
