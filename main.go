package main

import (
	"context"
	"encoding/base64"
	"encoding/binary"
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
	rpcURL    = "wss://solemn-fluent-glitter.solana-mainnet.quiknode.pro/a4b0c2d7fa048c4818a5f20dd20018d16ebdc4d3"
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
				// (TODO) SolanaBlockSubscribe version to hanlde legacy version
				// Error parsing block notification: json: cannot unmarshal string into Go struct field .params.result.value.block.transactions.version of type int
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

	// Print block details
	fmt.Println("\n=== Block Details ===")
	fmt.Printf("Block Number: %d\n", block.BlockHeight)
	fmt.Printf("Block Timestamp: %d\n", block.BlockTime)
	fmt.Printf("Block Hash: %s\n", block.Blockhash)

	// Process transactions
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
			decodeInstruction(instr, idl, versionedTransaction.Message)
		}

		// Attempt to deserialize as a legacy transaction (TODO)

		// Print transaction metadata
		fmt.Println("\n=== Transaction Metadata ===")
		fmt.Printf("Fee: %d\n", txn.Meta.Fee)
		fmt.Printf("Pre-Balances: %v\n", txn.Meta.PreBalances)
		fmt.Printf("Post-Balances: %v\n", txn.Meta.PostBalances)

		// Print token balances
		printTokenBalances("Pre-Token Balances", txn.Meta.PreTokenBalances)
		printTokenBalances("Post-Token Balances", txn.Meta.PostTokenBalances)

	}
}

// handleTransaction logs transaction details
func handleTransaction(tx *solana.Transaction) {
	signatures := []string{}
	for _, signature := range tx.Signatures {
		signatures = append(signatures, base58.Encode(signature[:]))
	}

	fmt.Println("\n=== Transaction Details ===")
	fmt.Printf("Signatures: %v\n", signatures)

	// txJSON, err := json.MarshalIndent(tx, "", "  ")
	// if err != nil {
	// 	log.Fatalf("Failed to marshal transaction to JSON: %v", err)
	// }
	// fmt.Println(string(txJSON))
}

func decodeInstruction(instr solana.CompiledInstruction, idl *IDLStructure, message solana.Message) {

	// Check if instruction data is long enough for discriminator
	if len(instr.Data) < 8 {
		fmt.Printf("Instruction data too short (length: %d)\n", len(instr.Data))
		fmt.Printf("Raw Data: %v\n", instr.Data)
		return
	}

	// Get discriminator
	discriminator := instr.Data[:8]
	fmt.Printf("Instruction Discriminator: %v\n", discriminator)

	// Match instruction by name or discriminator
	var matchedInstruction *struct {
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
	}

	// Match instruction
	for i, instruction := range idl.Instructions {
		fmt.Printf("Potential Instruction Match: %s\n", instruction.Name)
		matchedInstruction = &idl.Instructions[i]
		break
	}

	if matchedInstruction == nil {
		fmt.Println("No matching instruction found in IDL")
		return
	}

	fmt.Printf("Matched Instruction: %s\n", matchedInstruction.Name)

	// Decode account arguments
	fmt.Println("Account Arguments:")
	for i, accountIndex := range instr.Accounts {
		if i < len(matchedInstruction.Accounts) && int(accountIndex) < len(message.AccountKeys) {
			accountInfo := matchedInstruction.Accounts[i]
			accountKey := message.AccountKeys[accountIndex]

			fmt.Printf("  %s (Index %d): %s\n",
				accountInfo.Name,
				accountIndex,
				accountKey.String(),
			)

			fmt.Printf("    Mutable: %v, Signer: %v\n",
				accountInfo.IsMut,
				accountInfo.IsSigner,
			)
		}
	}

	// Decode instruction data
	fmt.Println("Instruction Data:")
	fmt.Printf("  Raw Data: %v\n", instr.Data)

	// Attempt to parse instruction arguments
	remainingData := instr.Data[8:] // Skip discriminator

	if len(matchedInstruction.Args) > 0 {
		fmt.Println("  Parsed Arguments:")

		for _, arg := range matchedInstruction.Args {
			if len(remainingData) == 0 {
				break
			}

			// Basic argument parsing with safety checks
			var value interface{}
			if len(remainingData) >= 8 {
				value = binary.LittleEndian.Uint64(remainingData[:8])
				remainingData = remainingData[8:]
			} else if len(remainingData) >= 4 {
				value = binary.LittleEndian.Uint32(remainingData[:4])
				remainingData = remainingData[4:]
			} else if len(remainingData) >= 1 {
				value = remainingData[0]
				remainingData = remainingData[1:]
			} else {
				value = "<no data>"
			}

			fmt.Printf("    %s: %v\n", arg.Name, value)
		}
	}

	// Print any remaining unparsed data
	if len(remainingData) > 0 {
		fmt.Printf("  Unparsed Data Remaining: %v\n", remainingData)
	}

	// Print documentation if available
	if len(matchedInstruction.Docs) > 0 {
		fmt.Println("Instruction Documentation:")
		for _, doc := range matchedInstruction.Docs {
			fmt.Println("  " + doc)
		}
	}
}

// printTokenBalances prints token balances for pre or post-transaction states
func printTokenBalances(label string, balances []TokenBalance) {
	if len(balances) > 0 {
		fmt.Printf("\n=== %s ===\n", label)
		for _, balance := range balances {
			fmt.Printf("Account Index: %d, Mint: %s, Owner: %s, UI Amount: %+v\n",
				balance.AccountIndex,
				balance.Mint,
				balance.Owner,
				balance.UIAmount,
			)
		}
	}
}
