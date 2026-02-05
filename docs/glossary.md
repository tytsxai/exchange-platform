# Glossary

Comprehensive glossary of cryptocurrency exchange terminology used in OpenExchange.

## 📖 A-E

### Ask
The lowest price at which sellers are willing to sell an asset. Also called "ask price" or "offer price".

### Balance
The amount of funds available in a user's account for a specific asset.

| Field | Description |
|-------|-------------|
| `available` | Funds available for trading |
| `frozen` | Funds locked in orders |

### Base Currency
The first currency in a trading pair (e.g., BTC in BTC_USDT).

### Bid
The highest price buyers are willing to pay for an asset. Also called "bid price".

### Block Chain
A distributed ledger technology that records all transactions across a network.

### Candlestick (K-Line)
A chart showing price movement over time, displaying open, high, low, and close prices.

### Clearing
The process of reconciling orders and transferring funds between buyers and sellers.

### CORS (Cross-Origin Resource Sharing)
A mechanism that allows restricted resources on a web page to be requested from another domain.

## 📖 F-J

### Fill
The execution of an order, either partially or completely.

| Fill Type | Description |
|-----------|-------------|
| `Full Fill` | Entire order quantity executed |
| `Partial Fill` | Only part of order quantity executed |

### Forex (FX)
Foreign exchange - trading one currency for another.

### GTC (Good Till Cancelled)
An order that remains active until it is filled or explicitly cancelled.

### Hash Rate
The speed at which a computer can complete computations in cryptocurrency mining.

### HFT (High-Frequency Trading)
Algorithmic trading characterized by high speed and high turnover.

### IOC (Immediate Or Cancel)
An order that must be executed immediately; any unfilled portion is cancelled.

## 📖 K-O

### KYC (Know Your Customer)
The process of verifying the identity of clients before allowing them to trade.

### Ledger
A record of all transactions and balance changes.

| Ledger Entry Types |
|-------------------|
| `TRADE` - Trade execution |
| `DEPOSIT` - Cryptocurrency deposit |
| `WITHDRAWAL` - Cryptocurrency withdrawal |
| `FEE` - Trading fee |
| `TRANSFER` - Internal transfer |

### Limit Order
An order to buy or sell at a specific price or better.

### Liquidity
The ability to buy or sell an asset without significantly affecting its price.

### Long
A bet that the price of an asset will rise.

### Maker
A trader who adds liquidity to the order book by placing limit orders.

### Margin
The collateral required to open or maintain a leveraged position.

### Market Order
An order to buy or sell immediately at the best available current price.

### Matching Engine
The system that pairs buy and sell orders to execute trades.

### Order
A request to buy or sell a specific quantity of an asset.

| Order States |
|--------------|
| `INIT` - Order created |
| `NEW` - Order submitted |
| `PARTIALLY_FILLED` - Partial execution |
| `FILLED` - Complete execution |
| `CANCELED` - User cancelled |
| `REJECTED` - Rejected by system |
| `EXPIRED` - Time limit reached |

## 📖 P-R

### Pair
A trading pair of two currencies (e.g., BTC/USDT).

### Position
The amount of an asset or currency held or owed.

### Post-Only
An order that only executes as a maker; if it would take liquidity, it's cancelled.

### Quote Currency
The second currency in a trading pair (e.g., USDT in BTC_USDT).

### Quote
The current price at which an asset can be bought or sold.

### Redis Streams
A message queue system used for event-driven communication between services.

## 📖 S-V

### Scalping
A trading strategy that profits from small price movements.

### Short
A bet that the price of an asset will fall.

### Slippage
The difference between the expected price of a trade and the executed price.

### Snowflake ID
A distributed unique ID generator used for order/trade IDs.

### Spread
The difference between the bid and ask prices.

| Spread Types |
|-------------|
| `Tight Spread` - Small difference, high liquidity |
| `Wide Spread` - Large difference, low liquidity |

### Stop Loss
An order to automatically sell when price falls to a specified level.

### Stop Limit
An order that becomes a limit order when a stop price is triggered.

### Symbol
A trading pair identifier (e.g., BTC_USDT).

### Taker
A trader who removes liquidity by executing market orders.

### Ticker
A summary of a trading pair's 24-hour performance.

### Time In Force
Instructions for how long an order remains active.

| TIF Options |
|------------|
| `GTC` - Good Till Cancelled |
| `IOC` - Immediate Or Cancel |
| `FOK` - Fill Or Kill |
| `POST_ONLY` - Maker only |

### Trade
The execution of a buy and sell order.

| Trade Attributes |
|----------------|
| `ID` - Unique trade identifier |
| `Price` - Execution price |
| `Quantity` - Amount traded |
| `Maker Order` - Limit order that provided liquidity |
| `Taker Order` - Market order that took liquidity |
| `Fee` - Trading fee charged |

### Volume
The total amount of an asset traded within a specific time period.

| Volume Types |
|-------------|
| `Trading Volume` - Total quantity traded |
| `Quote Volume` - Total value in quote currency |
| `24h Volume` - Volume over 24 hours |

## 📖 W-Z

### WebSocket
A communication protocol providing full-duplex communication channels over a single TCP connection.

### Whale
A trader with large positions that can affect market prices.

### Withdrawal
The process of moving funds from an exchange wallet to an external wallet.

| Withdrawal States |
|------------------|
| `PENDING` - Awaiting approval |
| `PROCESSING` - Being processed |
| `ON_CHAIN` - Submitted to blockchain |
| `COMPLETED` - Confirmed on blockchain |
| `FAILED` - Transaction failed |

### Zero Fill
When an order is completely filled at the quoted price without slippage.

## 🔗 Related Documentation

- [Architecture](architecture.md) - System design
- [API Documentation](api.md) - REST and WebSocket APIs
- [Trading Flow](trading-flow.md) - Order lifecycle
