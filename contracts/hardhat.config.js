{
  "solidity": {
    "version": "0.8.20",
    "optimizer": {
      "enabled": true,
      "runs": 200
    }
  },
  "contracts": {
    "AIBToken": {
      "path": "evm/AIBToken.sol",
      "args": []
    },
    "StakingRewards": {
      "path": "evm/StakingRewards.sol",
      "args": ["<AIBToken.address>", "10000000000000000"]
    },
    "Governance": {
      "path": "evm/Governance.sol",
      "args": ["<AIBToken.address>", "7200", "65", "10000000000000000000000", "10000000000000000000000000"]
    }
  },
  "networks": {
    "localhost": {
      "url": "http://localhost:8545",
      "accounts": [
        "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
      ]
    },
    "mainnet": {
      "url": "https://mainnet.example.com",
      "accounts": []
    }
  }
}
