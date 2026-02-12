package plugin

import "testing"

func TestConventionBasedURL(t *testing.T) {
	r := &Resolver{}
	got := r.conventionBasedURL("petstore-agent", "flokoa-system")
	want := "http://petstore-agent.flokoa-system.svc.cluster.local/"
	if got != want {
		t.Fatalf("unexpected convention URL: got=%q want=%q", got, want)
	}
}
