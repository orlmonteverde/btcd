// Copyright (c) 2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"encoding/binary"
	"fmt"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcutil"
	"github.com/btcsuite/btcutil/gcs"

	"os"
)

const (
	// cfIndexName is the human-readable name for the index.
	cfIndexName = "committed bloom filter index"
)

var (
	// cfIndexKey is the name of the db bucket used to house the
	// block hash -> CF index.
	cfIndexKey = []byte("cfbyhashidx")
)

func dbFetchCFIndexEntry(dbTx database.Tx, blockHash *chainhash.Hash) ([]byte,
    error) {
	// Load the record from the database and return now if it doesn't exist.
	index := dbTx.Metadata().Bucket(cfIndexKey)
	serializedFilter := index.Get(blockHash[:])
	if len(serializedFilter) == 0 {
		return nil, nil
	}

	return serializedFilter, nil
}

// The serialized format for keys and values in the block hash to CF bucket is:
//   <hash> = <CF>
//
//   Field           Type              Size
//   hash            chainhash.Hash    32 bytes
//   CF              []byte            variable
//   -----
//   Total: > 32 bytes

// CFIndex implements a CF by hash index.
type CFIndex struct {
	db database.DB
}

// Ensure the CFIndex type implements the Indexer interface.
var _ Indexer = (*CFIndex)(nil)

// Init initializes the hash-based CF index.
//
// This is part of the Indexer interface.
func (idx *CFIndex) Init() error {
	return nil
}

// Key returns the database key to use for the index as a byte slice.
//
// This is part of the Indexer interface.
func (idx *CFIndex) Key() []byte {
	return cfIndexKey
}

// Name returns the human-readable name of the index.
//
// This is part of the Indexer interface.
func (idx *CFIndex) Name() string {
	return cfIndexName
}

// Create is invoked when the indexer manager determines the index needs
// to be created for the first time.  It creates the buckets for the hash-based
// CF index.
//
// This is part of the Indexer interface.
func (idx *CFIndex) Create(dbTx database.Tx) error {
	meta := dbTx.Metadata()
	_, err := meta.CreateBucket(cfIndexKey)
	return err
}

func generateFilterForBlock(block *btcutil.Block) ([]byte, error) {
	txSlice := block.Transactions() // XXX can this fail?
	txHashes := make([][]byte, len(txSlice))

	for i := 0; i < len(txSlice); i++ {
		txHash, err := block.TxHash(i)
		if err != nil {
			return nil, err
		}
		txHashes = append(txHashes, txHash[:])
	}

	var key [gcs.KeySize]byte
	P := uint8(20) // collision probability

	for i := 0; i < gcs.KeySize; i += 4 {
		binary.BigEndian.PutUint32(key[i:], uint32(0xcafebabe))
	}

	filter, err := gcs.BuildGCSFilter(P, key, txHashes)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(os.Stderr, "Generated CF for block %v", block.Hash())

	return filter.Bytes(), nil
}

// ConnectBlock is invoked by the index manager when a new block has been
// connected to the main chain.  This indexer adds a hash-to-CF mapping for
// every passed block.
//
// This is part of the Indexer interface.
func (idx *CFIndex) ConnectBlock(dbTx database.Tx, block *btcutil.Block,
    view *blockchain.UtxoViewpoint) error {
	filterBytes, err := generateFilterForBlock(block)
	if err != nil {
		return err
	}

	meta := dbTx.Metadata()
	index := meta.Bucket(cfIndexKey)
	err = index.Put(block.Hash()[:], filterBytes)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Stored CF for block %v", block.Hash())

	return nil
}

// DisconnectBlock is invoked by the index manager when a block has been
// disconnected from the main chain.  This indexer removes the hash-to-CF
// mapping for every passed block.
//
// This is part of the Indexer interface.
func (idx *CFIndex) DisconnectBlock(dbTx database.Tx, block *btcutil.Block,
    view *blockchain.UtxoViewpoint) error {
	index := dbTx.Metadata().Bucket(cfIndexKey)
	filterBytes := index.Get(block.Hash()[:])
	if len(filterBytes) == 0 {
		return fmt.Errorf("can't remove non-existent filter %s from " +
		    "the cfilter index", block.Hash())
	}
	return index.Delete(block.Hash()[:])
}

func (idx *CFIndex) FilterByBlockHash(hash *chainhash.Hash) ([]byte, error) {
	var filterBytes []byte
	err := idx.db.View(func(dbTx database.Tx) error {
		var err error
		filterBytes, err = dbFetchCFIndexEntry(dbTx, hash)
		return err
	})
	return filterBytes, err
}

// NewCFIndex returns a new instance of an indexer that is used to create a
// mapping of the hashes of all blocks in the blockchain to their respective
// committed bloom filters.
//
// It implements the Indexer interface which plugs into the IndexManager that in
// turn is used by the blockchain package.  This allows the index to be
// seamlessly maintained along with the chain.
func NewCFIndex(db database.DB) *CFIndex {
	return &CFIndex{db: db}
}

// DropCFIndex drops the CF index from the provided database if exists.
func DropCFIndex(db database.DB) error {
	return dropIndex(db, cfIndexKey, cfIndexName)
}