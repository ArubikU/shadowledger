// Command slctl is the ShadowLedger wallet and chain inspection CLI.
package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/ArubikU/shadowledger/internal/crypto"
	"github.com/ArubikU/shadowledger/internal/types"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "keygen":
		keygen(os.Args[2:])
	case "address":
		address(os.Args[2:])
	case "balance":
		balance(os.Args[2:])
	case "send":
		send(os.Args[2:])
	case "head":
		head(os.Args[2:])
	case "block":
		block(os.Args[2:])
	case "reconstruct":
		reconstruct(os.Args[2:])
	case "supply":
		fmt.Println(getJSON(rpcOf(os.Args[2:]) + "/supply"))
	case "peers":
		fmt.Println(getJSON(rpcOf(os.Args[2:]) + "/peers"))
	case "deploy":
		deploy(os.Args[2:])
	case "call":
		call(os.Args[2:])
	default:
		usage()
	}
}

// deploy sends a contract-creation tx. --code is a path to a hex-encoded bytecode file.
func deploy(args []string) {
	rpc := rpcOf(args)
	kp, err := crypto.LoadWalletAuto(flagVal(args, "wallet"), passOf(args))
	must(err)
	code := readHexArg(args, "code")
	tx := types.Transaction{Kind: types.KindDeploy, Data: code, Gas: mustU64opt(args, "gas", 100000), Nonce: fetchNonce(rpc, kp.Address())}
	tx.Sign(kp)
	submit(rpc, tx)
	fmt.Printf("contract address: %s\n", types.ContractAddress(kp.Address(), tx.Nonce))
}

// call sends a contract-call tx. --to is the contract; --data hex words input (optional).
func call(args []string) {
	rpc := rpcOf(args)
	kp, err := crypto.LoadWalletAuto(flagVal(args, "wallet"), passOf(args))
	must(err)
	var data []byte
	if d := flagVal(args, "data"); d != "" {
		data = decodeHex(d)
	}
	amount := uint64(0)
	if a := flagVal(args, "amount"); a != "" {
		amount = mustU64(a)
	}
	tx := types.Transaction{
		To: crypto.Address(flagVal(args, "to")), Kind: types.KindCall,
		Amount: amount, Data: data, Gas: mustU64opt(args, "gas", 100000), Nonce: fetchNonce(rpc, kp.Address()),
	}
	tx.Sign(kp)
	submit(rpc, tx)
}

func usage() {
	fmt.Fprintln(os.Stderr, `slctl - ShadowLedger CLI

  keygen      --out wallet.tok [--pass P]   (.tok = encrypted; .json = plaintext)
  address     --wallet w.tok [--pass P]
  balance     --addr <addr> --rpc http://localhost:4004
  send        --wallet w.tok [--pass P] --to <addr> --amount N [--fee N] --rpc URL

Passphrase for .tok wallets: --pass or env SL_WALLET_PASS.
  head        --rpc URL
  block       --height N --rpc URL
  reconstruct --height N --rpc URL    (slow-path: rebuild body from shards)
  supply      --rpc URL               ($SHARD minted + next block reward)
  peers       --rpc URL               (this node's known peers)
  deploy      --wallet w.tok --code prog.hex [--gas N] --rpc URL   (deploy a contract)
  call        --wallet w.tok --to <contract> [--data HEX] [--amount N] [--gas N] --rpc URL`)
	os.Exit(2)
}

func flagVal(args []string, name string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--"+name {
			return args[i+1]
		}
	}
	return ""
}

// passOf returns the wallet passphrase from --pass or the SL_WALLET_PASS env var.
func passOf(args []string) string {
	if p := flagVal(args, "pass"); p != "" {
		return p
	}
	return os.Getenv("SL_WALLET_PASS")
}

func keygen(args []string) {
	out := flagVal(args, "out")
	if out == "" {
		out = "wallet.json"
	}
	kp, err := crypto.Generate()
	must(err)
	// .tok extension -> encrypted (requires a passphrase); .json -> plaintext.
	if err := kp.SaveWalletAuto(out, passOf(args)); err != nil {
		must(err)
	}
	enc := "plaintext"
	if crypto.IsTok(out) {
		enc = "encrypted (.tok)"
	}
	fmt.Printf("wallet written to %s [%s]\naddress: %s\n", out, enc, kp.Address())
}

func address(args []string) {
	kp, err := crypto.LoadWalletAuto(flagVal(args, "wallet"), passOf(args))
	must(err)
	fmt.Println(kp.Address())
}

func balance(args []string) {
	rpc := rpcOf(args)
	addr := flagVal(args, "addr")
	body := getJSON(rpc + "/account/" + addr)
	fmt.Println(body)
}

func head(args []string) {
	fmt.Println(getJSON(rpcOf(args) + "/head"))
}

func block(args []string) {
	fmt.Println(getJSON(rpcOf(args) + "/block/" + flagVal(args, "height")))
}

func reconstruct(args []string) {
	fmt.Println(getJSON(rpcOf(args) + "/reconstruct/" + flagVal(args, "height")))
}

func send(args []string) {
	rpc := rpcOf(args)
	kp, err := crypto.LoadWalletAuto(flagVal(args, "wallet"), passOf(args))
	must(err)
	to := crypto.Address(flagVal(args, "to"))
	amount := mustU64(flagVal(args, "amount"))
	fee := uint64(0)
	if f := flagVal(args, "fee"); f != "" {
		fee = mustU64(f)
	}
	// Fetch current nonce.
	var acct struct {
		Nonce uint64 `json:"nonce"`
	}
	must(json.Unmarshal([]byte(getJSON(rpc+"/account/"+string(kp.Address()))), &acct))

	tx := types.Transaction{To: to, Amount: amount, Fee: fee, Nonce: acct.Nonce}
	tx.Sign(kp)

	b, _ := json.Marshal(tx)
	resp, err := http.Post(rpc+"/tx", "application/json", bytes.NewReader(b))
	must(err)
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		fmt.Fprintf(os.Stderr, "send failed (%d): %s\n", resp.StatusCode, string(out))
		os.Exit(1)
	}
	fmt.Printf("sent: %s\n", string(out))
}

func rpcOf(args []string) string {
	rpc := flagVal(args, "rpc")
	if rpc == "" {
		rpc = "http://localhost:4004"
	}
	return rpc
}

func getJSON(url string) string {
	resp, err := http.Get(url)
	must(err)
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		fmt.Fprintf(os.Stderr, "request failed (%d): %s\n", resp.StatusCode, string(b))
		os.Exit(1)
	}
	return string(b)
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func mustU64(s string) uint64 {
	var v uint64
	if _, err := fmt.Sscanf(s, "%d", &v); err != nil {
		must(fmt.Errorf("bad number %q", s))
	}
	return v
}

func mustU64opt(args []string, name string, def uint64) uint64 {
	if v := flagVal(args, name); v != "" {
		return mustU64(v)
	}
	return def
}

func decodeHex(s string) []byte {
	b, err := hex.DecodeString(strings.TrimSpace(strings.TrimPrefix(s, "0x")))
	must(err)
	return b
}

func readHexArg(args []string, name string) []byte {
	path := flagVal(args, name)
	if path == "" {
		must(fmt.Errorf("--%s (hex bytecode file) required", name))
	}
	raw, err := os.ReadFile(path)
	must(err)
	return decodeHex(string(raw))
}

func fetchNonce(rpc string, addr crypto.Address) uint64 {
	var acct struct {
		Nonce uint64 `json:"nonce"`
	}
	must(json.Unmarshal([]byte(getJSON(rpc+"/account/"+string(addr))), &acct))
	return acct.Nonce
}

func submit(rpc string, tx types.Transaction) {
	b, _ := json.Marshal(tx)
	resp, err := http.Post(rpc+"/tx", "application/json", bytes.NewReader(b))
	must(err)
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		fmt.Fprintf(os.Stderr, "submit failed (%d): %s\n", resp.StatusCode, string(out))
		os.Exit(1)
	}
	fmt.Printf("submitted: %s\n", string(out))
}
