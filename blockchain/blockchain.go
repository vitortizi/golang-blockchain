package blockchain

import (
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"runtime"

	"github.com/dgraph-io/badger/v4"
)

const (
	dbPath      = "./tmp/blocks"
	dbFile      = "./tmp/blocks/MANIFEST"
	genesisData = "First Transaction from Genesis"
	lastHashKey = "LAST_HASH"
)

type Blockchain struct {
	LastHash []byte
	Database *badger.DB
}

type BlockchainIterator struct {
	CurrentHash []byte
	Database    *badger.DB
}

func DBexists() bool {
	if _, err := os.Stat(dbFile); os.IsNotExist(err) {
		return false
	}

	return true
}

func ContinueBlockchain(address string) *Blockchain {
	if DBexists() == false {
		fmt.Println("No existing Blockchain found, please create one!")
		runtime.Goexit()
	}

	var lastHash []byte

	opts := badger.DefaultOptions(dbPath)
	db, err := badger.Open(opts)
	if err != nil {
		log.Panic(err)
	}

	err = db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(lastHashKey))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			lastHash = append([]byte{}, val...) // cópia segura
			return nil
		})
	})

	if err != nil {
		log.Panic(err)
	}

	chain := Blockchain{LastHash: lastHash, Database: db}
	return &chain
}

func InitBlockchain(address string) *Blockchain {
	var lastHash []byte

	if DBexists() {
		fmt.Println("Blockchain already exists")
		runtime.Goexit()
	}

	opts := badger.DefaultOptions(dbPath)
	db, err := badger.Open(opts)
	if err != nil {
		log.Panic(err)
	}

	err = db.Update(func(txn *badger.Txn) error {
		cbtx := CoinBaseTx(address, genesisData)
		genesis := Genesis(cbtx)
		fmt.Println("Genesis Created")

		err = txn.Set(genesis.Hash, genesis.Serialize())
		if err != nil {
			log.Panic(err)
		}

		err = txn.Set([]byte(lastHashKey), genesis.Hash)
		lastHash = genesis.Hash

		return err
	})

	if err != nil {
		log.Panic(err)
	}

	return &Blockchain{LastHash: lastHash, Database: db}
}

func (chain *Blockchain) AddBlock(transactions []*Transaction) {
	var lastHash []byte

	err := chain.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(lastHashKey))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			lastHash = append([]byte{}, val...) // cópia segura
			return nil
		})
	})

	if err != nil {
		log.Panicf("failed to read last hash: %v", err)
	}

	newBlock := CreateBlock(transactions, lastHash)

	err = chain.Database.Update(func(txn *badger.Txn) error {
		if err := txn.Set(newBlock.Hash, newBlock.Serialize()); err != nil {
			return err
		}
		if err := txn.Set([]byte(lastHashKey), newBlock.Hash); err != nil {
			return err
		}

		chain.LastHash = newBlock.Hash
		return nil
	})

	if err != nil {
		log.Panicf("failed to add new block: %v", err)
	}
}

func (chain *Blockchain) Iterator() *BlockchainIterator {
	iter := &BlockchainIterator{chain.LastHash, chain.Database}

	return iter
}

func (iter *BlockchainIterator) Next() *Block {
	var block *Block
	var blockData []byte

	if err := iter.Database.View(func(txn *badger.Txn) error {
		item, err := txn.Get(iter.CurrentHash)
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			blockData = append([]byte{}, val...)
			return nil
		})
	}); err != nil {
		log.Panic(err)
	}

	block = Deserialize(blockData)
	iter.CurrentHash = block.PrevHash

	return block
}

func (chain *Blockchain) FindUnspentTransaction(address string) []Transaction {
	var unspentTxs []Transaction
	spentTxOs := make(map[string][]int)

	iter := chain.Iterator()

	for {
		block := iter.Next()
		for _, tx := range block.Transactions {
			txID := hex.EncodeToString(tx.ID)

		Outputs:
			for outIdx, out := range tx.Outputs {
				if spentTxOs[txID] != nil {
					for _, spentOut := range spentTxOs[txID] {
						if spentOut == outIdx {
							continue Outputs
						}
					}
				}

				if out.CanBeUnlocked(address) {
					unspentTxs = append(unspentTxs, *tx)
				}
			}

			if !tx.IsCoinBase() {
				for _, in := range tx.Inputs {
					if in.CanUnlock(address) {
						inTxID := hex.EncodeToString(in.ID)
						spentTxOs[inTxID] = append(spentTxOs[inTxID], in.Out)
					}
				}
			}
		}

		if len(block.PrevHash) == 0 {
			break
		}
	}

	return unspentTxs
}

func (chain *Blockchain) FindUTXOutput(address string) []TxOutput {
	var UTXOutput []TxOutput
	unspentTransactions := chain.FindUnspentTransaction(address)

	for _, tx := range unspentTransactions {
		for _, out := range tx.Outputs {
			if out.CanBeUnlocked(address) {
				UTXOutput = append(UTXOutput, out)
			}
		}
	}

	return UTXOutput
}

func (chain *Blockchain) FindSpendableOutputs(address string, amount int) (int, map[string][]int) {
	unspentOuts := make(map[string][]int)
	unspentTxs := chain.FindUnspentTransaction(address)
	accumulated := 0

Work:
	for _, tx := range unspentTxs {
		txID := hex.EncodeToString(tx.ID)

		for outIdx, out := range tx.Outputs {
			if out.CanBeUnlocked(address) && accumulated < amount {
				accumulated += out.Value
				unspentOuts[txID] = append(unspentOuts[txID], outIdx)

				if accumulated >= amount {
					break Work
				}
			}
		}
	}

	return accumulated, unspentOuts
}
