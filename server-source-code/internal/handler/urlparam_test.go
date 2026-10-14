package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestDecodedURLParamPackageID(t *testing.T) {
	r := chi.NewRouter()
	var got string
	r.Get("/packages/{packageId}", func(w http.ResponseWriter, r *http.Request) { got = decodedURLParam(r, "packageId") })

	cases := map[string]string{
		"/packages/2026-10%20Security%20Update%20(KB5127648)%20(26300.9711)": "2026-10 Security Update (KB5127648) (26300.9711)",
		"/packages/Mozilla%20Firefox%20(x64%20en-GB)":                        "Mozilla Firefox (x64 en-GB)",
		"/packages/x%20y":                                "x y",
		"/packages/libstdc++6":                           "libstdc++6",
		"/packages/libstdc%2B%2B6":                       "libstdc++6",
		"/packages/50%25":                                "50%",
		"/packages/c%2525weird":                          "c%25weird",
		"/packages/dir%2Fname":                           "dir/name",
		"/packages/a%23b":                                "a#b",
		"/packages/q%3Fx":                                "q?x",
		"/packages/%E5%90%8D%E5%89%8D":                   "名前",
		"/packages/3f2b6c1e-0000-4000-8000-000000000001": "3f2b6c1e-0000-4000-8000-000000000001",
	}
	for target, want := range cases {
		got = ""
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, target, nil))
		if got != want {
			t.Errorf("%s: got %q, want %q", target, got, want)
		}
	}
}

func TestDecodedURLParamRole(t *testing.T) {
	r := chi.NewRouter()
	var got string
	r.Put("/permissions/roles/{role}", func(w http.ResponseWriter, r *http.Request) { got = decodedURLParam(r, "role") })

	cases := map[string]string{
		"/permissions/roles/Ops%20Team%20(EU)":     "Ops Team (EU)",
		"/permissions/roles/Ops%20Team%20%28EU%29": "Ops Team (EU)",
		"/permissions/roles/Ops%20Team":            "Ops Team",
		"/permissions/roles/qa%2Fteam":             "qa/team",
		"/permissions/roles/host_manager":          "host_manager",
	}
	for target, want := range cases {
		got = ""
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPut, target, nil))
		if got != want {
			t.Errorf("%s: got %q, want %q", target, got, want)
		}
	}
}
