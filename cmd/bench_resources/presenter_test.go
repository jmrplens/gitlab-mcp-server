package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// presentingConn is a scripted client that can present another credential,
// recording every one it was asked to.
type presentingConn struct {
	scriptedConn
	mu     sync.Mutex
	seen   []credential
	refuse error
	closed atomic.Int64
}

func (c *presentingConn) callAs(_ context.Context, _ string, _ map[string]any, cred credential) ([]byte, error) {
	c.mu.Lock()
	c.seen = append(c.seen, cred)
	c.mu.Unlock()
	if c.refuse != nil {
		return nil, c.refuse
	}
	return []byte(`{"jsonrpc":"2.0","id":1,"result":{}}`), nil
}

func (c *presentingConn) close() { c.closed.Add(1) }

// presented is every credential the client was asked to present, in order.
func (c *presentingConn) presented() []credential {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.seen)
}

// floodPlan is a plan whose noisy population offers rate invented tokens a
// second from one credential.
func floodPlan(rate float64) fairnessPlan {
	return fairnessPlan{Noisy: populationSpec{Name: populationNoisy, Credentials: 1, Rate: rate, Verbs: []string{verbListInvented}}}
}

// TestSourceAddress_AndForwardedAddress_AreDistinctPerIndex verifies the two
// address sequences the flood is spread over: each index is its own address,
// and neither sequence starts on an address the host already means something
// by.
func TestSourceAddress_AndForwardedAddress_AreDistinctPerIndex(t *testing.T) {
	cases := []struct {
		name, got, want string
	}{
		{name: "the first source", got: sourceAddress(0), want: "127.2.0.1"},
		{name: "a source past the first octet", got: sourceAddress(256), want: "127.2.1.1"},
		{name: "the last source the range holds", got: sourceAddress(maxFloodSources - 1), want: "127.2.255.254"},
		{name: "the first invented token's address", got: forwardedAddress(1), want: "10.0.0.1"},
		{name: "an address past two octets", got: forwardedAddress(1<<16 + 2), want: "10.1.0.2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
}

// TestPresenter_MintsAndInventsDistinctCredentials verifies every credential
// handed out is one the server has never been shown, and that invented ones
// are taken from the sources in turn.
func TestPresenter_MintsAndInventsDistinctCredentials(t *testing.T) {
	sources := []*presentingConn{{}, {}, {}}
	p := &presenter{}
	for _, source := range sources {
		p.sources = append(p.sources, source)
	}

	first, err := p.mint()
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	second, _ := p.mint()
	if first == second || !strings.HasPrefix(first, newTokenPrefix) {
		t.Errorf("minted %q and %q, want two distinct new credentials", first, second)
	}

	var tokens, addresses []string
	for n := 1; n <= 4; n++ {
		source, cred, inventErr := p.invent()
		if inventErr != nil {
			t.Fatalf("invent: %v", inventErr)
		}
		if want := sources[n%len(sources)]; source != want {
			t.Errorf("invented token %d went through source %p, want %p", n, source, want)
		}
		if !strings.HasPrefix(cred.token, inventedTokenPrefix) {
			t.Errorf("invented %q, want the prefix the stand-in refuses", cred.token)
		}
		tokens = append(tokens, cred.token)
		addresses = append(addresses, cred.forwardedFor)
	}
	slices.Sort(tokens)
	slices.Sort(addresses)
	if len(slices.Compact(tokens)) != 4 || len(slices.Compact(addresses)) != 4 {
		t.Errorf("tokens %v from %v, want every token and every address distinct", tokens, addresses)
	}

	p.close()
	for index, source := range sources {
		t.Run("source "+strconv.Itoa(index), func(t *testing.T) {
			if source.closed.Load() != 1 {
				t.Errorf("closed %d times, want once", source.closed.Load())
			}
		})
	}
}

// TestPresenter_WithNothingToPresent_SaysSo verifies a presenter an arm never
// built, or one with no flood source, refuses rather than panicking: a verb
// that needs a credential in a plan that provides none is the harness wired
// wrong, and it is filed as a failure the run's own control then names.
func TestPresenter_WithNothingToPresent_SaysSo(t *testing.T) {
	var none *presenter
	if _, err := none.mint(); !errors.Is(err, errNoPresenter) {
		t.Errorf("mint on no presenter = %v, want %v", err, errNoPresenter)
	}
	if _, _, err := none.invent(); !errors.Is(err, errNoPresenter) {
		t.Errorf("invent on no presenter = %v, want %v", err, errNoPresenter)
	}
	if _, _, err := (&presenter{}).invent(); !errors.Is(err, errNoPresenter) {
		t.Errorf("invent with no source = %v, want %v", err, errNoPresenter)
	}
}

// TestNewPresenter_OneSource_IsTheDefaultRoute verifies a flood small enough
// to need one source is sent as any other client is, bearer or not, to the
// arm's own endpoint.
func TestNewPresenter_OneSource_IsTheDefaultRoute(t *testing.T) {
	p := newPresenter(floodPlan(1), "http://127.0.0.1:1/mcp", true)
	defer p.close()
	if len(p.sources) != 1 {
		t.Fatalf("sources = %d, want one", len(p.sources))
	}
	client, ok := p.sources[0].(*httpRPC)
	if !ok || !client.bearer || client.endpoint != "http://127.0.0.1:1/mcp" {
		t.Errorf("source = %+v, want a bearer client of the arm's endpoint", p.sources[0])
	}
	if got := newPresenter(fairnessPlan{Noisy: populationSpec{Credentials: 1, Rate: 1, Verbs: []string{verbList}}}, "", false); len(got.sources) != 0 {
		t.Errorf("a plan with no flood built %d sources, want none", len(got.sources))
	}
}

// TestNewPresenter_ManySources_SendEachFromItsOwnAddress verifies against a
// real listener that the flood's requests leave from the addresses its sources
// name, carrying the invented token and the forwarded address, which is what
// keeps the transport-source budget from seeing one proxy.
func TestNewPresenter_ManySources_SendEachFromItsOwnAddress(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("only Linux answers every address of 127.0.0.0/8 on its loopback without setup")
	}
	var mu sync.Mutex
	type seen struct{ host, auth, forwarded string }
	var requests []seen
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		mu.Lock()
		requests = append(requests, seen{host: host, auth: r.Header.Get("Authorization"), forwarded: r.Header.Get(headerForwardedFor)})
		mu.Unlock()
		w.Header().Set(headerContentType, mediaJSON)
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	}))
	defer server.Close()

	// Ten a second over a minute is six hundred distinct addresses, which is
	// three sources at two hundred and fifty each.
	plan := floodPlan(10)
	if got := plan.floodSources(); got != 3 {
		t.Fatalf("floodSources = %d, want three", got)
	}
	p := newPresenter(plan, server.URL+"/mcp", true)
	defer p.close()
	for range 3 {
		source, cred, err := p.invent()
		if err != nil {
			t.Fatalf("invent: %v", err)
		}
		if _, err = source.callAs(t.Context(), methodToolsList, nil, cred); err != nil {
			t.Fatalf("callAs: %v", err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	var hosts []string
	for _, request := range requests {
		hosts = append(hosts, request.host)
		if !strings.HasPrefix(request.auth, "Bearer "+inventedTokenPrefix) || !strings.HasPrefix(request.forwarded, "10.") {
			t.Errorf("request %+v, want an invented bearer token and a forwarded address", request)
		}
	}
	slices.Sort(hosts)
	if want := []string{"127.2.0.1", "127.2.0.2", "127.2.0.3"}; !slices.Equal(hosts, want) {
		t.Errorf("the flood left from %v, want %v", hosts, want)
	}
}

// presentCall is the tool call the present tests send.
func presentCall(t *testing.T) toolCall {
	t.Helper()
	call, err := callFor(surfaceDynamic)
	if err != nil {
		t.Fatalf("callFor: %v", err)
	}
	return call
}

// TestPresent_InventedToken_GoesThroughAFloodSource verifies an invented token
// is sent through the flood's sources with the forwarded address it claims,
// and that a plan that built no source says so.
func TestPresent_InventedToken_GoesThroughAFloodSource(t *testing.T) {
	call := presentCall(t)
	t.Run("through a flood source", func(t *testing.T) {
		source := &presentingConn{}
		in := paceInput{conn: &clientConn{rpc: &scriptedConn{}}, call: call, present: &presenter{sources: []floodSource{source}}}
		if err := present(t.Context(), in, verbs[verbListInvented]); err != nil {
			t.Fatalf("present: %v", err)
		}
		got := source.presented()
		if len(got) != 1 || !strings.HasPrefix(got[0].token, inventedTokenPrefix) || got[0].forwardedFor == "" {
			t.Errorf("presented %+v, want one invented token with its forwarded address", got)
		}
	})
	t.Run("with no source", func(t *testing.T) {
		in := paceInput{conn: &clientConn{rpc: &scriptedConn{}}, call: call}
		if err := present(t.Context(), in, verbs[verbListInvented]); !errors.Is(err, errNoPresenter) {
			t.Errorf("present = %v, want %v", err, errNoPresenter)
		}
	})
}

// TestPresent_NewCredential_IsAdoptedOnlyOnceServed verifies a new credential
// is minted and presented by the lane's own client, and adopted only once it
// was served.
//
// Adoption on a refused first presentation would have the lane present, as a
// cached credential, one the server never admitted, and every later request
// on it would be filed under the row that says the ceiling does not reach it.
func TestPresent_NewCredential_IsAdoptedOnlyOnceServed(t *testing.T) {
	call := presentCall(t)
	cannot := []struct {
		name string
		in   paceInput
		want error
	}{
		{
			name: "a client that cannot present one", want: errCannotPresent,
			in: paceInput{conn: &clientConn{rpc: &scriptedConn{}}, call: call, present: &presenter{}},
		},
		{
			name: "no presenter to mint it", want: errNoPresenter,
			in: paceInput{conn: &clientConn{rpc: &presentingConn{}}, call: call},
		},
	}
	for _, tc := range cannot {
		t.Run(tc.name, func(t *testing.T) {
			if err := present(t.Context(), tc.in, verbs[verbCallNew]); !errors.Is(err, tc.want) {
				t.Errorf("present = %v, want %v", err, tc.want)
			}
		})
	}
	t.Run("refused and not adopted", func(t *testing.T) {
		refused := errors.New("refused")
		conn := &clientConn{rpc: &presentingConn{refuse: refused}}
		if err := present(t.Context(), paceInput{conn: conn, call: call, present: &presenter{}}, verbs[verbCallNew]); !errors.Is(err, refused) {
			t.Errorf("present = %v, want the refusal returned", err)
		}
		if conn.current() != "" {
			t.Errorf("the lane adopted %q, a credential the server refused", conn.current())
		}
	})
	t.Run("served, adopted and then presented", func(t *testing.T) {
		client := &presentingConn{}
		in := paceInput{conn: &clientConn{rpc: client}, call: call, present: &presenter{}}
		if err := present(t.Context(), in, verbs[verbCallNew]); err != nil {
			t.Fatalf("present new: %v", err)
		}
		if err := present(t.Context(), in, verbs[verbCall]); err != nil {
			t.Fatalf("present own: %v", err)
		}
		got := client.presented()
		if len(got) != 2 || got[0].token != got[1].token || !strings.HasPrefix(got[0].token, newTokenPrefix) {
			t.Errorf("presented %+v, want the new credential twice, once new and once reused", got)
		}
		if client.calls.Load() != 0 {
			t.Errorf("the lane's own credential was presented %d times after it adopted a new one", client.calls.Load())
		}
	})
}

// TestPresent_OwnCredential_GoesOutAsItAlwaysDid verifies a lane that never
// adopted a credential sends its own, through the client's ordinary call.
func TestPresent_OwnCredential_GoesOutAsItAlwaysDid(t *testing.T) {
	client := &presentingConn{}
	if err := present(t.Context(), paceInput{conn: &clientConn{rpc: client}, call: presentCall(t)}, verbs[verbCall]); err != nil {
		t.Fatalf("present: %v", err)
	}
	if client.calls.Load() != 1 || len(client.presented()) != 0 {
		t.Errorf("own calls %d and presented %v, want one call with the client's own credential", client.calls.Load(), client.presented())
	}
}
