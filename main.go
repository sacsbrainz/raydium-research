package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	solana "github.com/gagliardetto/solana-go"
	"github.com/mr-tron/base58"
)

type IDLStructure struct {
	Version      string `json:"version"`
	Name         string `json:"name"`
	Instructions []struct {
		Name     string   `json:"name"`
		Docs     []string `json:"docs,omitempty"`
		Accounts []struct {
			Name     string   `json:"name"`
			IsMut    bool     `json:"isMut"`
			IsSigner bool     `json:"isSigner"`
			Docs     []string `json:"docs,omitempty"`
		} `json:"accounts,omitempty"`
		Args []struct {
			Name string `json:"name"`
		} `json:"args,omitempty"`
	} `json:"instructions"`
	Accounts []struct {
		Name string   `json:"name"`
		Docs []string `json:"docs,omitempty"`
	} `json:"accounts,omitempty"`
	Types []struct {
		Name string   `json:"name"`
		Docs []string `json:"docs,omitempty"`
	} `json:"types,omitempty"`
	Events []struct {
		Name   string `json:"name"`
		Fields []struct {
			Name string `json:"name"`
		} `json:"fields"`
	} `json:"events,omitempty"`
	Errors []struct {
		Code int    `json:"code"`
		Name string `json:"name"`
		Msg  string `json:"msg"`
	} `json:"errors,omitempty"`
}

// SolanaRPCRequest represents the JSON-RPC request structure
type SolanaRPCRequest struct {
	JsonRPC string        `json:"jsonrpc"`
	ID      int           `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
}

// SolanaBlockSubscribe represents the structure of the logs notification
type SolanaBlockSubscribe struct {
	JsonRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  struct {
		Result struct {
			Context struct {
				Slot int `json:"slot"`
			} `json:"context"`
			Value struct {
				Block *struct {
					BlockHeight       int    `json:"blockHeight"`
					BlockTime         int    `json:"blockTime"`
					Blockhash         string `json:"blockhash"`
					ParentSlot        int    `json:"parentSlot"`
					PreviousBlockhash string `json:"previousBlockhash"`
					Transactions      []struct {
						Transaction []string `json:"transaction"`
						Meta        struct {
							Err               interface{}    `json:"err"`
							Status            interface{}    `json:"status"`
							Fee               int            `json:"fee"`
							PreBalances       []int          `json:"preBalances"`
							PostBalances      []int          `json:"postBalances"`
							LogMessages       []string       `json:"logMessages"`
							PreTokenBalances  []TokenBalance `json:"preTokenBalances"`
							PostTokenBalances []TokenBalance `json:"postTokenBalances"`
						} `json:"meta"`
						Version int `json:"version"`
					} `json:"transactions"`
				} `json:"block"`
				Err *struct {
					InstructionError []interface{} `json:"InstructionError"`
				} `json:"err"`
				Slot int `json:"slot"`
			} `json:"value"`
		} `json:"result"`
		Subscription int `json:"subscription"`
	} `json:"params"`
}

type TokenBalance struct {
	AccountIndex int    `json:"accountIndex"`
	Mint         string `json:"mint"`
	Owner        string `json:"owner"`
	UIAmount     struct {
		UIAmount    *float64 `json:"uiAmount"`
		Decimals    int      `json:"decimals"`
		Amount      string   `json:"amount"`
		UIAmountStr string   `json:"uiAmountString"`
	} `json:"uiTokenAmount"`
}

const (
	programID = "CAMMCzo5YL8w4VFF8KVHrK22GGUsp5VTaW7grrKgrWqK"
	rpcURL    = "wss://solana-rpc.publicnode.com"
)

func main() {

	// Load IDL
	idl, err := loadIDL("idl.json")
	if err != nil {
		log.Fatalf("Failed to load IDL: %v", err)
	}

	// Create websocket connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, rpcURL, nil)
	if err != nil {
		log.Fatalf("Failed to connect to Solana RPC: %v", err)
	}
	defer conn.CloseNow()

	// Set a custom read limit
	conn.SetReadLimit(10 * 1024 * 1024) // 10 MB

	// Subscription request
	subscribeRequest := SolanaRPCRequest{
		JsonRPC: "2.0",
		ID:      1,
		Method:  "blockSubscribe",
		Params: []interface{}{
			map[string]string{
				"mentionsAccountOrProgram": programID,
			},
			map[string]interface{}{
				"commitment":                     "confirmed",
				"encoding":                       "base64",
				"showRewards":                    false,
				"transactionDetails":             "full",
				"maxSupportedTransactionVersion": 0,
			},
		},
	}

	// Send subscription request
	if err := wsjson.Write(ctx, conn, subscribeRequest); err != nil {
		log.Fatalf("Failed to send subscription request: %v", err)
	}

	// Channel to handle graceful shutdown
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	// Receive and process transactions
	go func() {
		for {
			var rawResponse json.RawMessage
			err := wsjson.Read(ctx, conn, &rawResponse)
			if err != nil {
				log.Printf("Error reading message: %v", err)
				return
			}

			// Parse the raw JSON
			var blockSubscribe SolanaBlockSubscribe
			if err := json.Unmarshal(rawResponse, &blockSubscribe); err != nil {
				log.Printf("Error parsing block notification: %v", err)
				continue
			}

			fmt.Println("\n============================")

			// Process transaction details
			if blockSubscribe.Params.Result.Value.Block != nil {
				processTransaction(blockSubscribe, idl)
				continue
			}

			// Incase they is an rpc error this will print it out
			json, err := json.MarshalIndent(rawResponse, "", "  ")
			if err != nil {
				panic(err)
			}

			fmt.Println(string(json))
		}
	}()

	// Wait for interrupt signal
	<-interrupt
	log.Println("Interrupt received, shutting down...")
}

// loadIDL reads and parses the IDL JSON file
func loadIDL(filepath string) (*IDLStructure, error) {
	file, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("error reading IDL file: %v", err)
	}

	var idl IDLStructure
	if err := json.Unmarshal(file, &idl); err != nil {
		return nil, fmt.Errorf("error parsing IDL JSON: %v", err)
	}

	return &idl, nil
}

// processTransaction extracts and prints detailed information from the transaction
func processTransaction(data SolanaBlockSubscribe, idl *IDLStructure) {

	block := data.Params.Result.Value.Block
	// Transaction basic information
	fmt.Println("=== Transaction Details ===")
	fmt.Printf("Block Number: %d\n", block.BlockHeight)
	fmt.Printf("Block Timestamp: %d\n", block.BlockTime)
	fmt.Printf("Block Hash: %s\n", block.Blockhash)

	// handle transaction
	for _, txn := range block.Transactions {

		// Decode the base64 transaction
		transactionBuffer, err := base64.StdEncoding.DecodeString((txn.Transaction[0]))
		if err != nil {
			log.Fatalf("Failed to decode base64 transaction: %v", err)
		}

		// Attempt to deserialize as a versioned transaction
		versionedTransaction, err := solana.TransactionFromBytes(transactionBuffer)
		if err == nil {
			// Handle versioned transaction
			handleTransaction(versionedTransaction)
		}

		// Decode instructions using the IDL
		for _, instr := range versionedTransaction.Message.Instructions {
			decodeInstruction(instr, idl, versionedTransaction.Message, solana.MustPublicKeyFromBase58(programID))
		}

		// Attempt to deserialize as a legacy transaction (TODO)

		// Transaction Metadata
		fmt.Printf("Transaction Fee: %d\n", txn.Meta.Fee)
		fmt.Printf("Pre-Balances: %v\n", txn.Meta.PreBalances)
		fmt.Printf("Post-Balances: %v\n", txn.Meta.PostBalances)

		// Token Balances
		fmt.Println("Pre-Token Balances:")
		for _, tb := range txn.Meta.PreTokenBalances {
			fmt.Printf("  Mint: %s, Owner: %s, Amount: %s\n", tb.Mint, tb.Owner, tb.UIAmount.UIAmountStr)
		}

		fmt.Println("Post-Token Balances:")
		for _, tb := range txn.Meta.PostTokenBalances {
			fmt.Printf("  Mint: %s, Owner: %s, Amount: %s\n", tb.Mint, tb.Owner, tb.UIAmount.UIAmountStr)
		}
	}
}

func handleTransaction(tx *solana.Transaction) {
	// Extract signatures and convert to base58
	var signatureStrings []string
	for _, signature := range tx.Signatures {
		signatureStrings = append(signatureStrings, base58.Encode(signature[:]))
	}

	// Log the signatures
	fmt.Println("Transaction Signatures:", signatureStrings)

	// log the transaction as JSON
	transactionJSON, err := json.MarshalIndent(tx, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal transaction to JSON: %v", err)
	}
	fmt.Println("Transaction Details:", string(transactionJSON))
}

func decodeInstruction(instr solana.CompiledInstruction, idl *IDLStructure, message solana.Message, programID solana.PublicKey) {
	_ = message
	_ = programID

	// Decode the instruction using the IDL
	for _, idlInstr := range idl.Instructions {

		fmt.Printf("Instruction Name: %s\n", idlInstr.Name)
		fmt.Printf("Raw Data: %v\n", instr.Data)

		// Parse arguments based on the IDL (TODO)
	}
}
