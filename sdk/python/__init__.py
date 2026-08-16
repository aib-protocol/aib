"""
AIB 2.0 SDK for Python

AIB 2.0 Specifications:
    - Cryptographic Algorithm: Ed25519
    - Address Format: Bech32m (HRP: "aib")
    - Transaction Model: UTXO

This package provides:
    - Wallet: Key management, signing, addresses
    - Transaction: UTXO transaction building and signing
    - Client: API client for blockchain interaction
"""

from .wallet import Wallet, Address
from .transaction import Transaction, TXInput, TXOutput, create_payment, create_batch_payment
from .client import Client, ClientConfig, send_payment, get_wallet_info

__version__ = "1.0.0"

__all__ = [
    "Wallet",
    "Address",
    "Transaction",
    "TXInput",
    "TXOutput",
    "Client",
    "ClientConfig",
    "create_payment",
    "create_batch_payment",
    "send_payment",
    "get_wallet_info",
]
