package vault_test

import (
	"context"
	"testing"
	"time"

	"github.com/keys-pub/keys-ext/vault"
	"github.com/stretchr/testify/require"
)

func TestInitRemoteDB(t *testing.T) {
	db1, close1 := newTestVaultDB(t)
	defer close1()
	db2, close2 := newTestVaultDB(t)
	defer close2()

	testInitRemote(t, db1, db2)
}

func TestInitRemoteMem(t *testing.T) {
	testInitRemote(t, vault.NewMem(), vault.NewMem())
}

func testInitRemote(t *testing.T, st1 vault.Store, st2 vault.Store) {
	// vault.SetLogger(vault.NewLogger(vault.DebugLevel))
	env := newTestEnv(t, nil) // vault.NewLogger(vault.DebugLevel))
	defer env.closeFn()

	// Client #1
	client1 := testClient(t, env)
	v1 := vault.New(st1)
	v1.SetRemote(client1)

	// Client #2
	client2 := testClient(t, env)
	v2 := vault.New(st2)
	v2.SetRemote(client2)

	var err error
	ctx := context.TODO()

	// Client #1
	err = v1.UnlockWithPassword("mypassword", true)
	require.NoError(t, err)

	provisions, err := v1.Provisions()
	require.NoError(t, err)
	require.Equal(t, 1, len(provisions))
	provision := provisions[0]

	err = v1.Set(vault.NewItem("key1", []byte("value1"), "", time.Now()))
	require.NoError(t, err)

	err = v1.Sync(ctx)
	require.NoError(t, err)

	// Client #2
	err = v2.InitRemote(ctx, v1.RemoteKey())
	require.NoError(t, err)

	err = v2.UnlockWithPassword("mypassword", false)
	require.NoError(t, err)

	out, err := v2.Get("key1")
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, "key1", out.ID)
	require.Equal(t, []byte("value1"), out.Data)

	paths1, err := v1.Paths("/pull")
	require.NoError(t, err)
	expected := []string{
		"/pull/000000000000001/config/salt",
		"/pull/000000000000002/auth/" + provision.ID,
		"/pull/000000000000003/provision/" + provision.ID,
		"/pull/000000000000004/item/key1",
	}
	require.Equal(t, expected, paths1)

	paths2, err := v2.Paths("/pull")
	require.NoError(t, err)
	require.Equal(t, expected, paths2)
}
