package pools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	serverapi "github.com/discobox-ai/discobox/api/gen"
	"github.com/discobox-ai/discobox/server/internal/model"
	services "github.com/discobox-ai/discobox/server/internal/services"
)

// A pool's host has a public URL unless the pool asks for none when it is
// created, which only a provider type whose validator takes it allows: a type
// that registers none gives its pools no public URL to withhold (#1077).
func TestCreatePoolWithoutAPublicURL(t *testing.T) {
	svc := newSizeFixture(t)
	ctx := context.Background()
	svc.providers.RegisterPoolValidator(sizedType, func(_ json.RawMessage, pool model.PoolManifest) error {
		if pool.DisablePublicURL && pool.SandboxOS != "linux" {
			return errors.New("the validator saw no OS")
		}
		return nil
	})

	pool, err := svc.CreatePool(ctx, "project-1", services.CreatePoolBody{Name: "public", ProviderInstanceId: "prov-sized"})
	if err != nil {
		t.Fatal(err)
	}
	if pool.DisablePublicURL {
		t.Fatal("a pool that asks nothing has no public URL, want one")
	}

	pool, err = svc.CreatePool(ctx, "project-1", services.CreatePoolBody{
		Name: "private", ProviderInstanceId: "prov-sized",
		DisablePublicUrl: serverapi.NewOptBool(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !pool.DisablePublicURL {
		t.Fatal("a pool that asks for no public URL was stored with one")
	}
	stored, err := svc.GetPool(ctx, "project-1", pool.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.DisablePublicURL {
		t.Fatal("the stored pool has a public URL")
	}

	_, err = svc.CreatePool(ctx, "project-1", services.CreatePoolBody{
		Name: "private-on-shared", ProviderInstanceId: "prov-unsized",
		DisablePublicUrl: serverapi.NewOptBool(true),
	})
	requireBadRequest(t, err, "Shared", "no public URL to withhold")
}
