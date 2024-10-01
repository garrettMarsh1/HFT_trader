package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
	"github.com/shopspring/decimal"
	pb "github.com/garrettMarsh1/open-bb-project/price_service"
	"google.golang.org/grpc"
)

const (
	pythStreamAddress = "localhost:50051"
	symbol            = "BTC/USD"
	tradeInterval     = 100 * time.Millisecond // 10 trades per second, adjust as needed
	stopLossPercent   = 0.5
	takeProfitPercent = 1.0
	positionSize      = 0.01 // 1% of available buying power
)

var (
	alpacaClient *alpaca.Client
	mdClient     *marketdata.Client
)

func init() {
	// Initialize Alpaca clients
	alpacaClient = alpaca.NewClient(alpaca.ClientOpts{})
	mdClient = marketdata.NewClient(marketdata.ClientOpts{})
}

func main() {
	// Set up gRPC connection to Pyth price service
	conn, err := grpc.Dial(pythStreamAddress, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewPriceServiceClient(conn)

	// Start streaming prices
	stream, err := client.StreamPrices(context.Background(), &pb.PriceRequest{})
	if err != nil {
		log.Fatalf("Error creating stream: %v", err)
	}

	ticker := time.NewTicker(tradeInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Receive price from Pyth
			resp, err := stream.Recv()
			if err != nil {
				log.Printf("Error receiving: %v", err)
				continue
			}

			// Get latest price from Alpaca
			alpacaPrice, err := getAlpacaPrice(symbol)
			if err != nil {
				log.Printf("Error getting Alpaca price: %v", err)
				continue
			}

			// Compare prices and generate signal
			signal := generateSignal(resp.Price, resp.ConfidenceInterval, alpacaPrice)

			// Execute trade based on signal
			if err := executeTrade(signal, decimal.NewFromFloat(resp.Price)); err != nil {
				log.Printf("Error executing trade: %v", err)
			}
		}
	}
}

func getAlpacaPrice(symbol string) (float64, error) {
	quote, err := mdClient.GetLatestCryptoQuote(symbol, marketdata.GetLatestCryptoQuoteRequest{})
	if err != nil {
		return 0, err
	}
	return quote.AskPrice, nil
}

func generateSignal(pythPrice, confidenceInterval, alpacaPrice float64) string {
	priceDiff := (pythPrice - alpacaPrice) / alpacaPrice
	if math.Abs(priceDiff) > confidenceInterval {
		if priceDiff > 0 {
			return "buy"
		}
		return "sell"
	}
	return "hold"
}

func executeTrade(signal string, currentPrice decimal.Decimal) error {
	account, err := alpacaClient.GetAccount()
	if err != nil {
		return fmt.Errorf("error getting account: %w", err)
	}

	buyingPower, err := account.BuyingPower.Float64()
	if err != nil {
		return fmt.Errorf("error converting buying power to float64: %w", err)
	}

	tradeValue := decimal.NewFromFloat(buyingPower * positionSize)
	quantity := tradeValue.Div(currentPrice)

	switch signal {
	case "buy":
		_, err = alpacaClient.PlaceOrder(alpaca.PlaceOrderRequest{
			Symbol:      symbol,
			Qty:         quantity,
			Side:        alpaca.Buy,
			Type:        alpaca.Market,
			TimeInForce: alpaca.GTC,
			OrderClass:  alpaca.Bracket,
			TakeProfit: &alpaca.TakeProfit{
				LimitPrice: currentPrice.Mul(decimal.NewFromFloat(1 + takeProfitPercent/100)).String(),
			},
			StopLoss: &alpaca.StopLoss{
				StopPrice: currentPrice.Mul(decimal.NewFromFloat(1 - stopLossPercent/100)).String(),
			},
		})
	case "sell":
		_, err = alpacaClient.PlaceOrder(alpaca.PlaceOrderRequest{
			Symbol:      symbol,
			Qty:         quantity,
			Side:        alpaca.Sell,
			Type:        alpaca.Market,
			TimeInForce: alpaca.GTC,
			OrderClass:  alpaca.Bracket,
			TakeProfit: &alpaca.TakeProfit{
				LimitPrice: currentPrice.Mul(decimal.NewFromFloat(1 - takeProfitPercent/100)).String(),
			},
			StopLoss: &alpaca.StopLoss{
				StopPrice: currentPrice.Mul(decimal.NewFromFloat(1 + stopLossPercent/100)).String(),
			},
		})
	}

	return err
}