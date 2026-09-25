package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/julienschmidt/httprouter"
)

func TestGETTrailingSlashCompatibility(t *testing.T) {
	router := httprouter.New()
	router.GET("/v1/events/guilds/:guild", func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		w.Header().Set("X-Query", r.URL.Query().Get("query"))
		w.WriteHeader(http.StatusOK)
	})
	router.GET("/v1/events/guilds/:guild/members", func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) { w.WriteHeader(http.StatusOK) })
	router.POST("/v1/events/guilds/:guild", func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) { w.WriteHeader(http.StatusCreated) })

	for _, tc := range []struct {
		method, url string
		status      int
		query       string
	}{
		{"GET", "/v1/events/guilds/173184118492889089", http.StatusOK, ""},
		{"GET", "/v1/events/guilds/173184118492889089/?query=test", http.StatusOK, "test"},
		{"GET", "/v1/events/guilds/173184118492889089/members/", http.StatusOK, ""},
		{"GET", "/v1/events/unknown/", http.StatusNotFound, ""},
		{"POST", "/v1/events/guilds/173184118492889089/", http.StatusTemporaryRedirect, ""},
	} {
		t.Run(tc.method+" "+tc.url, func(t *testing.T) {
			rec := httptest.NewRecorder()
			stateGETHandler(router).ServeHTTP(rec, httptest.NewRequest(tc.method, tc.url, nil))
			if rec.Code != tc.status {
				t.Fatalf("status = %d; want %d", rec.Code, tc.status)
			}
			if rec.Header().Get("X-Query") != tc.query {
				t.Fatalf("query = %q; want %q", rec.Header().Get("X-Query"), tc.query)
			}
		})
	}
}
