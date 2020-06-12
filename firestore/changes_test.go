package firestore

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/keys-pub/keys/ds"
	"github.com/stretchr/testify/require"
)

func TestFirestoreChanges(t *testing.T) {
	// SetContextLogger(NewContextLogger(DebugLevel))
	changes := testFirestore(t)
	ctx := context.TODO()
	col := ds.Path("changes", "test", testCollection())

	length := 40
	paths := []string{}
	values := []string{}
	for i := 0; i < length; i++ {
		value := fmt.Sprintf("value%d", i)
		path, err := changes.ChangeAdd(ctx, col, []byte(value))
		require.NoError(t, err)
		paths = append(paths, path)
		values = append(values, value)
	}

	// Changes (limit=10, asc)
	iter, err := changes.Changes(ctx, col, time.Time{}, 10, ds.Ascending)
	require.NoError(t, err)
	chgs, ts, err := ds.ChangesFromIterator(iter, time.Time{})
	require.NoError(t, err)
	iter.Release()
	require.Equal(t, 10, len(chgs))
	chgsValues := []string{}
	for _, doc := range chgs {
		chgsValues = append(chgsValues, string(doc.Data))
	}
	require.Equal(t, values[0:10], chgsValues)

	// Changes (ts, asc)
	iter, err = changes.Changes(ctx, col, ts, 10, ds.Ascending)
	require.NoError(t, err)
	chgs, ts, err = ds.ChangesFromIterator(iter, ts)
	require.NoError(t, err)
	iter.Release()
	require.False(t, ts.IsZero())
	require.Equal(t, 10, len(chgs))
	chgsValues = []string{}
	for _, doc := range chgs {
		chgsValues = append(chgsValues, string(doc.Data))
	}
	require.Equal(t, values[9:19], chgsValues)

	// Changes (now)
	now := time.Now()
	iter, err = changes.Changes(ctx, col, now, 100, ds.Ascending)
	require.NoError(t, err)
	chgs, ts, err = ds.ChangesFromIterator(iter, now)
	require.NoError(t, err)
	iter.Release()
	require.Equal(t, 0, len(chgs))
	require.Equal(t, now, ts)

	// Descending
	revValues := reverseCopy(values)

	// Changes (limit=10, desc)
	iter, err = changes.Changes(ctx, col, time.Time{}, 10, ds.Descending)
	require.NoError(t, err)
	chgs, ts, err = ds.ChangesFromIterator(iter, time.Time{})
	require.NoError(t, err)
	iter.Release()
	require.Equal(t, 10, len(chgs))
	require.False(t, ts.IsZero())
	chgsValues = []string{}
	for _, doc := range chgs {
		chgsValues = append(chgsValues, string(doc.Data))
	}
	require.Equal(t, revValues[0:10], chgsValues)

	// Changes (limit=5, ts, desc)
	iter, err = changes.Changes(ctx, col, ts, 5, ds.Descending)
	require.NoError(t, err)
	chgs, ts, err = ds.ChangesFromIterator(iter, ts)
	require.NoError(t, err)
	iter.Release()
	require.Equal(t, 5, len(chgs))
	require.False(t, ts.IsZero())
	chgsValues = []string{}
	for _, doc := range chgs {
		chgsValues = append(chgsValues, string(doc.Data))
	}
	require.Equal(t, revValues[9:14], chgsValues)
}

func stringsCopy(s []string) []string {
	a := make([]string, len(s))
	copy(a, s)
	return a
}

func reverseCopy(s []string) []string {
	a := make([]string, len(s))
	for i, j := 0, len(s)-1; i < len(s); i++ {
		a[i] = s[j]
		j--
	}
	return a
}
