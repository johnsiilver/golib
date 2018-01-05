package trees

import (

)

// Base defines basic tree methods common to all trees.
type Base interface {
  Search(ctx context.Context, key keys.Key) (interface{}, error)
  Insert(ctx context.Context, key keys.Key, value interface{}) error
  Delete(ctx context.Context, key keys.Key)
  NumEntries() (uint64, error)
}
