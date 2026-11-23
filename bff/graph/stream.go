package graph

import (
	"encoding/json"
	"fmt"
	"net/http"

	graphql "github.com/graph-gophers/graphql-go"
)

// StreamHandler serves GraphQL subscriptions as Server-Sent Events.
//
// The client POSTs a normal GraphQL request body ({"query", "variables",
// "operationName"}) and reads a text/event-stream response: one
// "event: next" per result, then "event: complete" when the subscription ends.
// This follows the "distinct connections" mode of the GraphQL over SSE
// protocol, using only the standard library.
func StreamHandler(schema *graphql.Schema) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query         string         `json:"query"`
			OperationName string         `json:"operationName"`
			Variables     map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		results, err := schema.Subscribe(r.Context(), req.Query, req.OperationName, req.Variables)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no") // tell nginx not to buffer
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		for res := range results {
			data, err := json.Marshal(res)
			if err != nil {
				data, _ = json.Marshal(map[string]any{"errors": []map[string]string{{"message": err.Error()}}})
			}
			fmt.Fprintf(w, "event: next\ndata: %s\n\n", data)
			flusher.Flush()
		}
		fmt.Fprint(w, "event: complete\ndata:\n\n")
		flusher.Flush()
	})
}
