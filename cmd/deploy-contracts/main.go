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

// Config 配置结构
type Config struct {
	RPCEndpoint string `yaml:"rpc_endpoint"`
	ChainID     int64  `yaml:"chain_id"`
	PrivateKey  string `yaml:"private_key"`
	GasPrice    string `yaml:"gas_price"`
	GasLimit    uint64 `yaml:"gas_limit"`
	Timeout     int    `yaml:"timeout_sec"`
}

// DeploymentRecord 部署记录
type DeploymentRecord struct {
	Timestamp   time.Time       `json:"timestamp"`
	Network     string          `json:"network"`
	ChainID     int64           `json:"chain_id"`
	Deployer    string          `json:"deployer"`
	Contracts   []ContractInfo  `json:"contracts"`
	Validations []ValidationInfo `json:"validations"`
}

// ContractInfo 合约信息
type ContractInfo struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	TxHash  string `json:"tx_hash"`
	GasUsed uint64 `json:"gas_used"`
}

// ValidationInfo 验证信息
type ValidationInfo struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// DeploymentResult 部署结果
type DeploymentResult struct {
	Address common.Address
	TxHash  common.Hash
	Receipt *types.Receipt
	Error   error
}

var (
	configFile = flag.String("config", "config.yaml", "配置文件路径")
	contract   = flag.String("contract", "all", "要部署的合约: weth, factory, router, all")
	network    = flag.String("network", "devnet", "网络: devnet, testnet, mainnet")
	outputDir  = flag.String("output", "./deployments", "输出目录")
	skipVerify = flag.Bool("skip-verify", false, "跳过验证")
	verbose    = flag.Bool("verbose", false, "详细输出")
)

func main() {
	flag.Parse()

	if *verbose {
		fmt.Printf("=== AIB 合约部署工具 ===\n")
		fmt.Printf("配置文件: %s\n", *configFile)
		fmt.Printf("网络: %s\n", *network)
		fmt.Printf("合约: %s\n\n", *contract)
	}

	// 加载配置
	cfg, err := loadConfig(*configFile)
	if err != nil {
		logError("加载配置失败: %v", err)
		os.Exit(1)
	}

	// 创建输出目录
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		logError("创建输出目录失败: %v", err)
		os.Exit(1)
	}

	// 连接节点
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Timeout)*time.Second)
	defer cancel()

	client, err := ethclient.DialContext(ctx, cfg.RPCEndpoint)
	if err != nil {
		logError("连接RPC节点失败: %v", err)
		os.Exit(1)
	}
	defer client.Close()

	// 获取私钥
	privateKey, err := crypto.HexToECDSA(strings.TrimPrefix(cfg.PrivateKey, "0x"))
	if err != nil {
		logError("解析私钥失败: %v", err)
		os.Exit(1)
	}

	// 获取发送者地址
	senderAddress := crypto.PubkeyToAddress(privateKey.PublicKey)
	if *verbose {
		fmt.Printf("部署地址: %s\n", senderAddress.Hex())
	}

	// 获取链ID
	chainID := big.NewInt(cfg.ChainID)

	// 初始化部署器
	deployer := NewDeployer(client, privateKey, chainID, cfg)

	// 执行部署
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
		feeTo := senderAddress // 默认手续费接收者
		results["UniswapV2Factory"], err = deployer.DeployFactory(ctx, feeTo)
	case "router":
		// Router需要Factory和WETH地址
		factoryAddr := getInputAddress("输入Factory地址:")
		wethAddr := getInputAddress("输入WETH地址:")
		results["UniswapV2Router"], err = deployer.DeployRouter(ctx, factoryAddr, wethAddr)
	case "all":
		logInfo("部署全部合约...")

		// 1. 部署WETH
		logInfo("1. 部署WETH...")
		results["WETH"], _ = deployer.DeployWETH(ctx)
		if results["WETH"].Error != nil {
			logError("WETH部署失败: %v", results["WETH"].Error)
		} else {
			logSuccess("WETH部署成功: %s", results["WETH"].Address.Hex())
		}

		// 2. 部署Factory
		logInfo("2. 部署UniswapV2Factory...")
		feeTo := senderAddress
		results["UniswapV2Factory"], _ = deployer.DeployFactory(ctx, feeTo)
		if results["UniswapV2Factory"].Error != nil {
			logError("Factory部署失败: %v", results["UniswapV2Factory"].Error)
		} else {
			logSuccess("Factory部署成功: %s", results["UniswapV2Factory"].Address.Hex())
		}

		// 3. 部署Router
		if results["WETH"].Error == nil && results["UniswapV2Factory"].Error == nil {
			logInfo("3. 部署UniswapV2Router...")
			results["UniswapV2Router"], err = deployer.DeployRouter(
				ctx,
				results["UniswapV2Factory"].Address,
				results["WETH"].Address,
			)
			if err != nil {
				logError("Router部署失败: %v", err)
			} else {
				logSuccess("Router部署成功: %s", results["UniswapV2Router"].Address.Hex())
			}
		}
	default:
		logError("未知合约类型: %s", *contract)
		os.Exit(1)
	}

	// 收集部署结果
	for name, result := range results {
		if result.Error != nil {
			logError("%s 部署失败: %v", name, result.Error)
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

	// 验证
	if !*skipVerify && len(record.Contracts) > 0 {
		logInfo("\n=== 开始验证 ===")
		validator := NewValidator(client, deployer)

		for _, contractInfo := range record.Contracts {
			logInfo("验证 %s...", contractInfo.Name)

			v := ValidationInfo{
				Name:      contractInfo.Name,
				Timestamp: time.Now(),
			}

			if err := validator.ValidateContract(ctx, contractInfo.Address); err != nil {
				v.Status = "failed"
				v.Message = err.Error()
				logError("  验证失败: %v", err)
			} else {
				v.Status = "success"
				v.Message = "合约验证通过"
				logSuccess("  验证通过")
			}

			record.Validations = append(record.Validations, v)
		}
	}

	// 保存部署记录
	recordFile := filepath.Join(*outputDir, fmt.Sprintf("deployment_%s_%d.json", *network, time.Now().Unix()))
	data, _ := json.MarshalIndent(record, "", "  ")
	if err := os.WriteFile(recordFile, data, 0644); err != nil {
		logError("保存部署记录失败: %v", err)
	} else {
		logSuccess("\n部署记录已保存: %s", recordFile)
	}

	// 生成合约地址映射文件
	if len(record.Contracts) > 0 {
		addrFile := filepath.Join(*outputDir, fmt.Sprintf("addresses_%s.txt", *network))
		var addrData strings.Builder
		addrData.WriteString(fmt.Sprintf("# AIB DeFi 合约地址映射\n"))
		addrData.WriteString(fmt.Sprintf("# 网络: %s\n", *network))
		addrData.WriteString(fmt.Sprintf("# 部署时间: %s\n\n", record.Timestamp.Format(time.RFC3339)))

		for _, c := range record.Contracts {
			addrData.WriteString(fmt.Sprintf("%s=%s\n", c.Name, c.Address))
		}

		if err := os.WriteFile(addrFile, []byte(addrData.String()), 0644); err != nil {
			logError("保存地址映射失败: %v", err)
		} else {
			logSuccess("地址映射已保存: %s", addrFile)
		}
	}

	logInfo("\n=== 部署完成 ===")
	for _, c := range record.Contracts {
		fmt.Printf("  %s: %s\n", c.Name, c.Address)
	}
}

// Deployer 部署器
type Deployer struct {
	client    *ethclient.Client
	privateKey *ecdsa.PrivateKey
	chainID   *big.Int
	sender    common.Address
	config    *Config
}

func NewDeployer(client *ethclient.Client, privateKey *ecdsa.PrivateKey, chainID *big.Int, cfg *Config) *Deployer {
	return &Deployer{
		client:    client,
		privateKey: privateKey,
		chainID:   chainID,
		sender:    crypto.PubkeyToAddress(privateKey.PublicKey),
		config:    cfg,
	}
}

// DeployWETH 部署WETH合约
func (d *Deployer) DeployWETH(ctx context.Context) (*DeploymentResult, error) {
	// WETH合约字节码
	bytecode, err := d.loadContractBytecode("WETH")
	if err != nil {
		return nil, fmt.Errorf("加载WETH字节码失败: %w", err)
	}

	return d.deploy(ctx, bytecode, nil)
}

// DeployFactory 部署UniswapV2Factory合约
func (d *Deployer) DeployFactory(ctx context.Context, feeTo common.Address) (*DeploymentResult, error) {
	bytecode, err := d.loadContractBytecode("UniswapV2Factory")
	if err != nil {
		return nil, fmt.Errorf("加载Factory字节码失败: %w", err)
	}

	// 构造函数参数: address _feeToSetter
	abiData, err := d.loadContractABI("UniswapV2Factory")
	if err != nil {
		return nil, fmt.Errorf("加载Factory ABI失败: %w", err)
	}

	parsedABI, err := abi.JSON(strings.NewReader(abiData))
	if err != nil {
		return nil, fmt.Errorf("解析ABI失败: %w", err)
	}

	constructor := parsedABI.Constructor
	input, err := constructor.Inputs.Pack(feeTo)
	if err != nil {
		return nil, fmt.Errorf("打包构造函数参数失败: %w", err)
	}

	return d.deploy(ctx, bytecode, input)
}

// DeployRouter 部署UniswapV2Router合约
func (d *Deployer) DeployRouter(ctx context.Context, factory, weth common.Address) (*DeploymentResult, error) {
	bytecode, err := d.loadContractBytecode("UniswapV2Router")
	if err != nil {
		return nil, fmt.Errorf("加载Router字节码失败: %w", err)
	}

	// 构造函数参数: address _factory, address _WETH
	abiData, err := d.loadContractABI("UniswapV2Router")
	if err != nil {
		return nil, fmt.Errorf("加载Router ABI失败: %w", err)
	}

	parsedABI, err := abi.JSON(strings.NewReader(abiData))
	if err != nil {
		return nil, fmt.Errorf("解析ABI失败: %w", err)
	}

	constructor := parsedABI.Constructor
	input, err := constructor.Inputs.Pack(factory, weth)
	if err != nil {
		return nil, fmt.Errorf("打包构造函数参数失败: %w", err)
	}

	return d.deploy(ctx, bytecode, input)
}

// deploy 通用部署函数
func (d *Deployer) deploy(ctx context.Context, bytecode []byte, input []byte) (*DeploymentResult, error) {
	result := &DeploymentResult{}

	// 构建交易数据
	var data []byte
	data = append(data, bytecode...)
	data = append(data, input...)

	// 获取当前nonce
	nonce, err := d.client.PendingNonceAt(ctx, d.sender)
	if err != nil {
		result.Error = fmt.Errorf("获取nonce失败: %w", err)
		return result, result.Error
	}

	// 解析gas价格
	gasPrice := new(big.Int)
	if d.config.GasPrice != "" {
		gasPrice, _ = new(big.Int).SetString(d.config.GasPrice, 0)
	} else {
		gasPrice, err = d.client.SuggestGasPrice(ctx)
		if err != nil {
			result.Error = fmt.Errorf("获取gas价格失败: %w", err)
			return result, result.Error
		}
	}

	// 设置gas限制
	gasLimit := d.config.GasLimit
	if gasLimit == 0 {
		gasLimit = 8000000 // 默认值
	}

	// 构建交易
	tx := types.NewTransaction(nonce, common.Address{}, big.NewInt(0), gasLimit, gasPrice, data)

	// 签名交易
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(d.chainID), d.privateKey)
	if err != nil {
		result.Error = fmt.Errorf("签名交易失败: %w", err)
		return result, result.Error
	}

	// 发送交易
	if err := d.client.SendTransaction(ctx, signedTx); err != nil {
		result.Error = fmt.Errorf("发送交易失败: %w", err)
		return result, result.Error
	}

	result.TxHash = signedTx.Hash()
	logInfo("交易已发送: %s", result.TxHash.Hex())

	// 等待交易确认
	receipt, err := bind.WaitMined(ctx, d.client, signedTx)
	if err != nil {
		result.Error = fmt.Errorf("等待交易确认失败: %w", err)
		return result, result.Error
	}

	if receipt.Status == 0 {
		result.Error = fmt.Errorf("交易执行失败")
		return result, result.Error
	}

	result.Address = *crypto.CreateAddress(d.sender, nonce)
	result.Receipt = receipt

	return result, nil
}

// loadContractBytecode 加载合约字节码
func (d *Deployer) loadContractBytecode(name string) ([]byte, error) {
	// 从contracts目录加载编译后的字节码
	bytecodeFile := filepath.Join("./contracts", name+".bin")

	data, err := os.ReadFile(bytecodeFile)
	if err != nil {
		return nil, err
	}

	return hex.DecodeString(strings.TrimSpace(string(data)))
}

// loadContractABI 加载合约ABI
func (d *Deployer) loadContractABI(name string) (string, error) {
	abiFile := filepath.Join("./contracts", name+".abi")

	data, err := os.ReadFile(abiFile)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// Validator 验证器
type Validator struct {
	client  *ethclient.Client
	deployer *Deployer
}

func NewValidator(client *ethclient.Client, deployer *Deployer) *Validator {
	return &Validator{
		client:  client,
		deployer: deployer,
	}
}

// ValidateContract 验证合约
func (v *Validator) ValidateContract(ctx context.Context, address common.Address) error {
	// 检查合约代码
	code, err := v.client.CodeAt(ctx, address, nil)
	if err != nil {
		return fmt.Errorf("获取合约代码失败: %w", err)
	}

	if len(code) == 0 {
		return fmt.Errorf("合约不存在")
	}

	logInfo("  合约代码长度: %d 字节", len(code))

	// TODO: 添加更多验证逻辑
	// - 验证合约函数
	// - 验证初始状态
	// - 运行测试用例

	return nil
}

// loadConfig 加载配置文件
func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		// 返回默认配置
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

// getInputAddress 从用户输入获取地址
func getInputAddress(prompt string) common.Address {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print(prompt)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	return common.HexToAddress(input)
}

// 日志函数
func logInfo(format string, args ...interface{}) {
	fmt.Printf("[INFO] %s\n", fmt.Sprintf(format, args...))
}

func logSuccess(format string, args ...interface{}) {
	fmt.Printf("[SUCCESS] %s\n", fmt.Sprintf(format, args...))
}

func logError(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[ERROR] %s\n", fmt.Sprintf(format, args...))
}
