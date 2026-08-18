package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aib-protocol/aib/pkg/evm"
	"github.com/aib-protocol/aib/pkg/evm/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"gopkg.in/yaml.v3"
)

// Config configuration struct
type Config struct {
	RPCEndpoint string `yaml:"rpc_endpoint"`
	ChainID     int64  `yaml:"chain_id"`
	PrivateKey  string `yaml:"private_key"`
	GasPrice    string `yaml:"gas_price"`
	GasLimit    uint64 `yaml:"gas_limit"`
	Timeout     int    `yaml:"timeout_sec"`
}

// DeploymentRecord deployment record
type DeploymentRecord struct {
	Timestamp   time.Time        `json:"timestamp"`
	Network     string           `json:"network"`
	ChainID     int64            `json:"chain_id"`
	Deployer    string           `json:"deployer"`
	Contracts   []ContractInfo   `json:"contracts"`
	Validations []ValidationInfo `json:"validations"`
}

// ContractInfo contract info
type ContractInfo struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	TxHash  string `json:"tx_hash"`
	GasUsed uint64 `json:"gas_used"`
}

// ValidationInfo validation info
type ValidationInfo struct {
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// DeploymentResult deployment result
type DeploymentResult struct {
	Address common.Address
	TxHash  common.Hash
	Receipt *types.Receipt
	Error   error
}

var (
	configFile = flag.String("config", "config.yaml", "path to config file")
	contract   = flag.String("contract", "all", "contract to deploy: weth, factory, router, all")
	network    = flag.String("network", "devnet", "network: devnet, testnet, mainnet")
	outputDir  = flag.String("output", "./deployments", "output directory")
	skipVerify = flag.Bool("skip-verify", false, "skip verification")
	verbose    = flag.Bool("verbose", false, "verbose output")
)

func main() {
	flag.Parse()

	if *verbose {
		fmt.Printf("=== AIB Contract Deployment Tool ===\n")
		fmt.Printf("Config file: %s\n", *configFile)
		fmt.Printf("Network: %s\n", *network)
		fmt.Printf("Contract: %s\n\n", *contract)
	}

	// load config
	cfg, err := loadConfig(*configFile)
	if err != nil {
		logError("failed to load config: %v", err)
		os.Exit(1)
	}

	// create output directory
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		logError("failed to create output directory: %v", err)
		os.Exit(1)
	}

	// connect to node
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Timeout)*time.Second)
	defer cancel()

	client, err := ethclient.DialContext(ctx, cfg.RPCEndpoint)
	if err != nil {
		logError("failed to connect to RPC node: %v", err)
		os.Exit(1)
	}
	defer client.Close()

	// get private key
	privateKey, err := crypto.HexToECDSA(strings.TrimPrefix(cfg.PrivateKey, "0x"))
	if err != nil {
		logError("failed to parse private key: %v", err)
		os.Exit(1)
	}

	// get sender address
	senderAddress := crypto.PubkeyToAddress(privateKey.PublicKey)
	if *verbose {
		fmt.Printf("Deploy address: %s\n", senderAddress.Hex())
	}

	// get chain ID
	chainID := big.NewInt(cfg.ChainID)

	// initialize deployer
	deployer := NewDeployer(client, privateKey, chainID, cfg)

	// execute deployment
	record := &DeploymentRecord{
		Timestamp:   time.Now(),
		Network:     *network,
		ChainID:     cfg.ChainID,
		Deployer:    senderAddress.Hex(),
		Contracts:   []ContractInfo{},
		Validations: []ValidationInfo{},
	}

	results := make(map[string]*DeploymentResult)

	switch *contract {
	case "weth":
		results["WETH"], err = deployer.DeployWETH(ctx)
	case "factory":
		feeTo := senderAddress // default fee recipient
		results["UniswapV2Factory"], err = deployer.DeployFactory(ctx, feeTo)
	case "router":
		// Router requires Factory and WETH addresses
		factoryAddr := getInputAddress("Enter Factory address:")
		wethAddr := getInputAddress("Enter WETH address:")
		results["UniswapV2Router"], err = deployer.DeployRouter(ctx, factoryAddr, wethAddr)
	case "all":
		logInfo("Deploying all contracts...")

		// 1. deploy WETH
		logInfo("1. Deploying WETH...")
		results["WETH"], _ = deployer.DeployWETH(ctx)
		if results["WETH"].Error != nil {
			logError("WETH deployment failed: %v", results["WETH"].Error)
		} else {
			logSuccess("WETH deployed successfully: %s", results["WETH"].Address.Hex())
		}

		// 2. deploy Factory
		logInfo("2. Deploying UniswapV2Factory...")
		feeTo := senderAddress
		results["UniswapV2Factory"], _ = deployer.DeployFactory(ctx, feeTo)
		if results["UniswapV2Factory"].Error != nil {
			logError("Factory deployment failed: %v", results["UniswapV2Factory"].Error)
		} else {
			logSuccess("Factory deployed successfully: %s", results["UniswapV2Factory"].Address.Hex())
		}

		// 3. deploy Router
		if results["WETH"].Error == nil && results["UniswapV2Factory"].Error == nil {
			logInfo("3. Deploying UniswapV2Router...")
			results["UniswapV2Router"], err = deployer.DeployRouter(
				ctx,
				results["UniswapV2Factory"].Address,
				results["WETH"].Address,
			)
			if err != nil {
				logError("Router deployment failed: %v", err)
			} else {
				logSuccess("Router deployed successfully: %s", results["UniswapV2Router"].Address.Hex())
			}
		}
	default:
		logError("unknown contract type: %s", *contract)
		os.Exit(1)
	}

	// collect deployment results
	for name, result := range results {
		if result.Error != nil {
			logError("%s deployment failed: %v", name, result.Error)
			continue
		}

		info := ContractInfo{
			Name:    name,
			Address: result.Address.Hex(),
			TxHash:  result.TxHash.Hex(),
		}
		if result.Receipt != nil {
			info.GasUsed = result.Receipt.GasUsed
		}
		record.Contracts = append(record.Contracts, info)
	}

	// verify
	if !*skipVerify && len(record.Contracts) > 0 {
		logInfo("\n=== Starting verification ===")
		validator := NewValidator(client, deployer)

		for _, contractInfo := range record.Contracts {
			logInfo("Verifying %s...", contractInfo.Name)

			v := ValidationInfo{
				Name:      contractInfo.Name,
				Timestamp: time.Now(),
			}

			if err := validator.ValidateContract(ctx, contractInfo.Address); err != nil {
				v.Status = "failed"
				v.Message = err.Error()
				logError("  verification failed: %v", err)
			} else {
				v.Status = "success"
				v.Message = "contract verification passed"
				logSuccess("  verification passed")
			}

			record.Validations = append(record.Validations, v)
		}
	}

	// save deployment record
	recordFile := filepath.Join(*outputDir, fmt.Sprintf("deployment_%s_%d.json", *network, time.Now().Unix()))
	data, _ := json.MarshalIndent(record, "", "  ")
	if err := os.WriteFile(recordFile, data, 0644); err != nil {
		logError("failed to save deployment record: %v", err)
	} else {
		logSuccess("\nDeployment record saved: %s", recordFile)
	}

	// generate contract address mapping file
	if len(record.Contracts) > 0 {
		addrFile := filepath.Join(*outputDir, fmt.Sprintf("addresses_%s.txt", *network))
		var addrData strings.Builder
		addrData.WriteString(fmt.Sprintf("# AIB DeFi contract address mapping\n"))
		addrData.WriteString(fmt.Sprintf("# Network: %s\n", *network))
		addrData.WriteString(fmt.Sprintf("# Deployed at: %s\n\n", record.Timestamp.Format(time.RFC3339)))

		for _, c := range record.Contracts {
			addrData.WriteString(fmt.Sprintf("%s=%s\n", c.Name, c.Address))
		}

		if err := os.WriteFile(addrFile, []byte(addrData.String()), 0644); err != nil {
			logError("failed to save address mapping: %v", err)
		} else {
			logSuccess("Address mapping saved: %s", addrFile)
		}
	}

	logInfo("\n=== Deployment complete ===")
	for _, c := range record.Contracts {
		fmt.Printf("  %s: %s\n", c.Name, c.Address)
	}
}

// Deployer deploys contracts
type Deployer struct {
	client     *ethclient.Client
	privateKey *ecdsa.PrivateKey
	chainID    *big.Int
	sender     common.Address
	config     *Config
}

func NewDeployer(client *ethclient.Client, privateKey *ecdsa.PrivateKey, chainID *big.Int, cfg *Config) *Deployer {
	return &Deployer{
		client:     client,
		privateKey: privateKey,
		chainID:    chainID,
		sender:     crypto.PubkeyToAddress(privateKey.PublicKey),
		config:     cfg,
	}
}

// DeployWETH deploys the WETH contract
func (d *Deployer) DeployWETH(ctx context.Context) (*DeploymentResult, error) {
	// WETH contract bytecode
	bytecode, err := d.loadContractBytecode("WETH")
	if err != nil {
		return nil, fmt.Errorf("failed to load WETH bytecode: %w", err)
	}

	return d.deploy(ctx, bytecode, nil)
}

// DeployFactory deploys the UniswapV2Factory contract
func (d *Deployer) DeployFactory(ctx context.Context, feeTo common.Address) (*DeploymentResult, error) {
	bytecode, err := d.loadContractBytecode("UniswapV2Factory")
	if err != nil {
		return nil, fmt.Errorf("failed to load Factory bytecode: %w", err)
	}

	// constructor args: address _feeToSetter
	abiData, err := d.loadContractABI("UniswapV2Factory")
	if err != nil {
		return nil, fmt.Errorf("failed to load Factory ABI: %w", err)
	}

	parsedABI, err := abi.JSON(strings.NewReader(abiData))
	if err != nil {
		return nil, fmt.Errorf("failed to parse ABI: %w", err)
	}

	constructor := parsedABI.Constructor
	input, err := constructor.Inputs.Pack(feeTo)
	if err != nil {
		return nil, fmt.Errorf("failed to pack constructor args: %w", err)
	}

	return d.deploy(ctx, bytecode, input)
}

// DeployRouter deploys the UniswapV2Router contract
func (d *Deployer) DeployRouter(ctx context.Context, factory, weth common.Address) (*DeploymentResult, error) {
	bytecode, err := d.loadContractBytecode("UniswapV2Router")
	if err != nil {
		return nil, fmt.Errorf("failed to load Router bytecode: %w", err)
	}

	// constructor args: address _factory, address _WETH
	abiData, err := d.loadContractABI("UniswapV2Router")
	if err != nil {
		return nil, fmt.Errorf("failed to load Router ABI: %w", err)
	}

	parsedABI, err := abi.JSON(strings.NewReader(abiData))
	if err != nil {
		return nil, fmt.Errorf("failed to parse ABI: %w", err)
	}

	constructor := parsedABI.Constructor
	input, err := constructor.Inputs.Pack(factory, weth)
	if err != nil {
		return nil, fmt.Errorf("failed to pack constructor args: %w", err)
	}

	return d.deploy(ctx, bytecode, input)
}

// deploy generic deployment function
func (d *Deployer) deploy(ctx context.Context, bytecode []byte, input []byte) (*DeploymentResult, error) {
	result := &DeploymentResult{}

	// build transaction data
	var data []byte
	data = append(data, bytecode...)
	data = append(data, input...)

	// get current nonce
	nonce, err := d.client.PendingNonceAt(ctx, d.sender)
	if err != nil {
		result.Error = fmt.Errorf("failed to get nonce: %w", err)
		return result, result.Error
	}

	// parse gas price
	gasPrice := new(big.Int)
	if d.config.GasPrice != "" {
		gasPrice, _ = new(big.Int).SetString(d.config.GasPrice, 0)
	} else {
		gasPrice, err = d.client.SuggestGasPrice(ctx)
		if err != nil {
			result.Error = fmt.Errorf("failed to get gas price: %w", err)
			return result, result.Error
		}
	}

	// set gas limit
	gasLimit := d.config.GasLimit
	if gasLimit == 0 {
		gasLimit = 8000000 // default value
	}

	// build transaction
	tx := types.NewTransaction(nonce, common.Address{}, big.NewInt(0), gasLimit, gasPrice, data)

	// sign transaction
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(d.chainID), d.privateKey)
	if err != nil {
		result.Error = fmt.Errorf("failed to sign transaction: %w", err)
		return result, result.Error
	}

	// send transaction
	if err := d.client.SendTransaction(ctx, signedTx); err != nil {
		result.Error = fmt.Errorf("failed to send transaction: %w", err)
		return result, result.Error
	}

	result.TxHash = signedTx.Hash()
	logInfo("Transaction sent: %s", result.TxHash.Hex())

	// wait for transaction confirmation
	receipt, err := bind.WaitMined(ctx, d.client, signedTx)
	if err != nil {
		result.Error = fmt.Errorf("failed to wait for transaction confirmation: %w", err)
		return result, result.Error
	}

	if receipt.Status == 0 {
		result.Error = fmt.Errorf("transaction execution failed")
		return result, result.Error
	}

	result.Address = *crypto.CreateAddress(d.sender, nonce)
	result.Receipt = receipt

	return result, nil
}

// loadContractBytecode loads contract bytecode
func (d *Deployer) loadContractBytecode(name string) ([]byte, error) {
	// load compiled bytecode from contracts directory
	bytecodeFile := filepath.Join("./contracts", name+".bin")

	data, err := os.ReadFile(bytecodeFile)
	if err != nil {
		return nil, err
	}

	return hex.DecodeString(strings.TrimSpace(string(data)))
}

// loadContractABI loads contract ABI
func (d *Deployer) loadContractABI(name string) (string, error) {
	abiFile := filepath.Join("./contracts", name+".abi")

	data, err := os.ReadFile(abiFile)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// Validator validates contracts
type Validator struct {
	client   *ethclient.Client
	deployer *Deployer
}

func NewValidator(client *ethclient.Client, deployer *Deployer) *Validator {
	return &Validator{
		client:   client,
		deployer: deployer,
	}
}

// ValidateContract validates a contract
func (v *Validator) ValidateContract(ctx context.Context, address common.Address) error {
	// check contract code
	code, err := v.client.CodeAt(ctx, address, nil)
	if err != nil {
		return fmt.Errorf("failed to get contract code: %w", err)
	}

	if len(code) == 0 {
		return fmt.Errorf("contract does not exist")
	}

	logInfo("  Contract code length: %d bytes", len(code))

	// TODO: add more validation logic
	// - verify contract functions
	// - verify initial state
	// - run test cases

	return nil
}

// loadConfig loads the config file
func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		// return default config
		return &Config{
			RPCEndpoint: "http://localhost:8545",
			ChainID:     314159,
			GasLimit:    8000000,
			Timeout:     300,
		}, nil
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// getInputAddress reads an address from user input
func getInputAddress(prompt string) common.Address {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print(prompt)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	return common.HexToAddress(input)
}

// logging functions
func logInfo(format string, args ...interface{}) {
	fmt.Printf("[INFO] %s\n", fmt.Sprintf(format, args...))
}

func logSuccess(format string, args ...interface{}) {
	fmt.Printf("[SUCCESS] %s\n", fmt.Sprintf(format, args...))
}

func logError(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[ERROR] %s\n", fmt.Sprintf(format, args...))
}
