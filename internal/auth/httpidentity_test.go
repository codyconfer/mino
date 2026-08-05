package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/codyconfer/mino/internal/errs"
)

func deviceServer(t *testing.T, h http.HandlerFunc) string {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestGitHubDeviceStartReturnsTheCodes(t *testing.T) {
	url := deviceServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.FormValue("client_id"); got != "cid" {
			t.Errorf("client_id = %q, want cid", got)
		}
		if _, ok := r.Form["scope"]; ok {
			t.Error("an empty scope was still sent; resolving a login needs no scope at all")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"device_code":"dc","user_code":"UC-1","verification_uri":"https://x/device","expires_in":900,"interval":5}`))
	})
	st, err := GitHubDeviceStart(context.Background(), url, "cid", "")
	if err != nil {
		t.Fatalf("GitHubDeviceStart: %v", err)
	}
	if st.DeviceCode != "dc" || st.UserCode != "UC-1" {
		t.Errorf("got %+v, want the codes from the response", st)
	}
	if st.Interval != 5*time.Second || st.ExpiresIn != 15*time.Minute {
		t.Errorf("interval = %s expires_in = %s, want 5s/15m", st.Interval, st.ExpiresIn)
	}
}

func TestGitHubDeviceStartClampsHostileTimings(t *testing.T) {
	url := deviceServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"device_code":"dc","user_code":"UC","expires_in":86400,"interval":0}`))
	})
	st, err := GitHubDeviceStart(context.Background(), url, "cid", "")
	if err != nil {
		t.Fatalf("GitHubDeviceStart: %v", err)
	}
	if st.Interval < minDevicePollInterval {
		t.Errorf("interval = %s, want at least %s; interval 0 would license a spin",
			st.Interval, minDevicePollInterval)
	}
	if st.ExpiresIn > maxDeviceLifetime {
		t.Errorf("expires_in = %s, want at most %s; a day-long expiry would pin a pending slot",
			st.ExpiresIn, maxDeviceLifetime)
	}
}

func TestGitHubDeviceStartNeedsAClientID(t *testing.T) {
	if _, err := GitHubDeviceStart(context.Background(), "http://unused", "  ", ""); err == nil {
		t.Fatal("an empty client id was accepted")
	} else if errs.KindOf(err) != errs.KindConfig {
		t.Errorf("kind = %s, want config", errs.KindOf(err))
	}
}

func TestGitHubDevicePollMapsEveryProtocolOutcome(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       DevicePoll
	}{
		{"authorized", `{"access_token":"gho_x","scope":""}`, DevicePoll{AccessToken: "gho_x"}},
		{"pending", `{"error":"authorization_pending"}`, DevicePoll{Pending: true}},
		{"slow down", `{"error":"slow_down"}`, DevicePoll{Pending: true, SlowDown: true}},
		{"denied", `{"error":"access_denied"}`, DevicePoll{Denied: true}},
		{"expired", `{"error":"expired_token"}`, DevicePoll{Expired: true}},
		{"empty body", `{}`, DevicePoll{Pending: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			url := deviceServer(t, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			})
			got, err := GitHubDevicePoll(context.Background(), url, "cid", "dc")
			if err != nil {
				t.Fatalf("GitHubDevicePoll: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestDeviceFlowDisabledIsAConfigError(t *testing.T) {
	for _, code := range []string{"device_flow_disabled", "unauthorized_client", "invalid_client"} {
		url := deviceServer(t, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"error":"` + code + `"}`))
		})
		_, err := GitHubDevicePoll(context.Background(), url, "cid", "dc")
		if err == nil {
			t.Fatalf("%s was accepted", code)
		}
		if errs.KindOf(err) != errs.KindConfig {
			t.Errorf("%s mapped to kind %s, want config; a 502 would leave the operator debugging "+
				"the forge for a setting they own", code, errs.KindOf(err))
		}
	}
}

func TestDevicePollNeverRelaysTheResponseBody(t *testing.T) {
	url := deviceServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"error":"something_odd","error_description":"gho_leaked_secret"}`))
	})
	_, err := GitHubDevicePoll(context.Background(), url, "cid", "dc")
	if err == nil {
		t.Fatal("an unknown error code was accepted")
	}
	if strings.Contains(err.Error(), "gho_leaked_secret") {
		t.Errorf("the error relayed the response body, which can carry a token and lands in the "+
			"log dir: %v", err)
	}
}

func TestDeviceResponseBodiesAreBounded(t *testing.T) {
	url := deviceServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		big := strings.Repeat("a", maxTokenResponseBytes+1024)
		_, _ = w.Write([]byte(`{"device_code":"` + big + `"}`))
	})
	if _, err := GitHubDeviceStart(context.Background(), url, "cid", ""); err == nil {
		t.Error("an oversize body was accepted; sisyphus's own device flow does an unbounded ReadAll, " +
			"which is why this one does not reuse it")
	}
}

func TestGitHubWhoAmIReturnsTheIdentity(t *testing.T) {
	url := deviceServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			t.Errorf("path = %q, want /user", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer gho_x" {
			t.Errorf("authorization = %q, want the device-flow token", got)
		}
		_, _ = w.Write([]byte(`{"login":"cody","id":4242,"type":"User"}`))
	})
	id, err := GitHubWhoAmI(context.Background(), url, "gho_x")
	if err != nil {
		t.Fatalf("GitHubWhoAmI: %v", err)
	}
	if id.Login != "cody" || id.ID != 4242 || id.Type != "User" {
		t.Errorf("got %+v, want cody/4242/User", id)
	}
}

func TestGitHubWhoAmIIgnoresAmbientCredentials(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_ambient")
	t.Setenv("GH_TOKEN", "ghp_ambient_two")
	url := deviceServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer gho_caller" {
			t.Errorf("authorization = %q; resolving identity through the ambient credential would "+
				"authenticate the machine and hand a session to whoever asked", got)
		}
		_, _ = w.Write([]byte(`{"login":"cody","id":1,"type":"User"}`))
	})
	if _, err := GitHubWhoAmI(context.Background(), url, "gho_caller"); err != nil {
		t.Fatalf("GitHubWhoAmI: %v", err)
	}
}

func TestGitHubWhoAmIClassifiesUpstreamRejections(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		hdr    map[string]string
		want   errs.Kind
	}{
		{"unauthorized", http.StatusUnauthorized, nil, errs.KindAuth},
		{"rate limited", http.StatusForbidden, map[string]string{"X-RateLimit-Remaining": "0"}, errs.KindSignal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			url := deviceServer(t, func(w http.ResponseWriter, _ *http.Request) {
				for k, v := range tc.hdr {
					w.Header().Set(k, v)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(`{"message":"nope"}`))
			})
			_, err := GitHubWhoAmI(context.Background(), url, "gho_x")
			if err == nil {
				t.Fatal("the rejection was accepted")
			}
			if errs.KindOf(err) != tc.want {
				t.Errorf("kind = %s, want %s", errs.KindOf(err), tc.want)
			}
		})
	}
}

func TestGitHubWhoAmIRejectsAnEmptyIdentity(t *testing.T) {
	url := deviceServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})
	if _, err := GitHubWhoAmI(context.Background(), url, "gho_x"); err == nil {
		t.Error("an empty login was accepted; it would then be compared against the allow-list")
	}
}

func TestOAuthEndpointsFollowTheAPIURL(t *testing.T) {
	for _, tc := range []struct{ name, apiURL, wantDevice string }{
		{"default", "", githubDeviceCodeURL},
		{"dotcom", "https://api.github.com", githubDeviceCodeURL},
		{"enterprise", "https://ghe.example.com/api/v3", "https://ghe.example.com/login/device/code"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			device, token, err := GitHubOAuthEndpoints(tc.apiURL)
			if err != nil {
				t.Fatalf("GitHubOAuthEndpoints: %v", err)
			}
			if device != tc.wantDevice {
				t.Errorf("device url = %q, want %q; the device endpoints live on the web host, not the "+
					"API host, so an enterprise deployment would otherwise send its users to github.com",
					device, tc.wantDevice)
			}
			if !strings.HasSuffix(token, "/login/oauth/access_token") {
				t.Errorf("token url = %q, want a /login/oauth/access_token endpoint", token)
			}
		})
	}
}

func TestOAuthEndpointsRefuseCleartext(t *testing.T) {
	if _, _, err := GitHubOAuthEndpoints("http://ghe.example.com/api/v3"); err == nil {
		t.Error("a cleartext api_url was accepted; the device code and the access token both cross it")
	} else if errs.KindOf(err) != errs.KindConfig {
		t.Errorf("kind = %s, want config", errs.KindOf(err))
	}
}

func TestARejectedClientIDIsAConfigError(t *testing.T) {
	url := deviceServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"Not Found"}`))
	})
	_, err := GitHubDeviceStart(context.Background(), url, "bogus", "")
	if err == nil {
		t.Fatal("a rejected client id was accepted")
	}
	if errs.KindOf(err) != errs.KindConfig {
		t.Errorf("kind = %s, want config; real GitHub answers an unknown client id with a bare 404, "+
			"and a 502 would send the operator to debug connectivity for a setting they own",
			errs.KindOf(err))
	}
}

func TestUpstreamServerErrorsStayUpstream(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusBadGateway} {
		url := deviceServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":"try_later"}`))
		})
		_, err := GitHubDeviceStart(context.Background(), url, "cid", "")
		if err == nil {
			t.Fatalf("status %d was accepted", status)
		}
		if errs.KindOf(err) != errs.KindSignal {
			t.Errorf("status %d mapped to kind %s, want signal; that one really is the forge's problem",
				status, errs.KindOf(err))
		}
	}
}

func TestVerificationFallbackFollowsTheDeviceHost(t *testing.T) {
	got := verificationFallback("https://ghe.example.com/login/device/code")
	if got != "https://ghe.example.com/login/device" {
		t.Errorf("fallback = %q, want the enterprise host; sending an enterprise user to github.com "+
			"would be a dead end", got)
	}
}
