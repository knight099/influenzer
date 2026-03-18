package razorpay

import (
	"errors"
	"fmt"

	"github.com/razorpay/razorpay-go"
	rzpUtils "github.com/razorpay/razorpay-go/utils"
	"github.com/vaibhaw/influenzer-backend/config"
)

type Client interface {
	CreateOrder(amount float64, currency string, receipt string, notes map[string]interface{}) (string, error)
	VerifyPaymentSignature(orderID, paymentID, signature string) error
	TransferFunds(accountID string, amount float64, currency string, notes map[string]interface{}) (string, error)
	CreateSubscription(planID string, totalCount int, customerNotify int, notes map[string]interface{}) (subscriptionID string, shortURL string, err error)
	// Razorpay X Payout methods
	CreateContact(name, email, phone, contactType string) (contactID string, err error)
	CreateFundAccount(contactID, accountHolderName, accountNumber, ifsc string) (fundAccountID string, err error)
	CreatePayout(accountNumber, fundAccountID string, amount float64, currency, purpose string, notes map[string]interface{}) (payoutID string, err error)
}

type razorpayClient struct {
	client        *razorpay.Client
	keySecret     string
	accountNumber string // Razorpay X account number for payouts
}

func NewRazorpayClient(cfg *config.Config) Client {
	client := razorpay.NewClient(cfg.RazorpayKeyID, cfg.RazorpayKeySecret)
	return &razorpayClient{
		client:        client,
		keySecret:     cfg.RazorpayKeySecret,
		accountNumber: cfg.RazorpayAccountNumber,
	}
}

func (r *razorpayClient) CreateOrder(amount float64, currency string, receipt string, notes map[string]interface{}) (string, error) {
	data := map[string]interface{}{
		"amount":          amount * 100,
		"currency":        currency,
		"receipt":         receipt,
		"notes":           notes,
		"payment_capture": 1,
	}
	body, err := r.client.Order.Create(data, nil)
	if err != nil {
		return "", fmt.Errorf("razorpay create order failed: %v", err)
	}

	orderID, ok := body["id"].(string)
	if !ok {
		return "", errors.New("failed to parse order id")
	}
	return orderID, nil
}

func (r *razorpayClient) VerifyPaymentSignature(orderID, paymentID, signature string) error {
	params := map[string]interface{}{
		"razorpay_order_id":   orderID,
		"razorpay_payment_id": paymentID,
	}
	if !rzpUtils.VerifyPaymentSignature(params, signature, r.keySecret) {
		return errors.New("invalid payment signature")
	}
	return nil
}

func (r *razorpayClient) TransferFunds(accountID string, amount float64, currency string, notes map[string]interface{}) (string, error) {
	data := map[string]interface{}{
		"account":  accountID,
		"amount":   amount * 100,
		"currency": currency,
		"notes":    notes,
		"on_hold":  0,
	}
	body, err := r.client.Transfer.Create(data, nil)
	if err != nil {
		return "", fmt.Errorf("razorpay transfer failed: %v", err)
	}

	transferID, ok := body["id"].(string)
	if !ok {
		return "", errors.New("failed to parse transfer id")
	}
	return transferID, nil
}

func (r *razorpayClient) CreateContact(name, email, phone, contactType string) (string, error) {
	data := map[string]interface{}{
		"name":    name,
		"type":    contactType,
		"email":   email,
		"contact": phone,
	}
	body, err := r.client.Post("/v1/contacts", data, nil)
	if err != nil {
		return "", fmt.Errorf("razorpay create contact failed: %v", err)
	}
	contactID, ok := body["id"].(string)
	if !ok {
		return "", errors.New("failed to parse contact id")
	}
	return contactID, nil
}

func (r *razorpayClient) CreateFundAccount(contactID, accountHolderName, accountNumber, ifsc string) (string, error) {
	data := map[string]interface{}{
		"contact_id":   contactID,
		"account_type": "bank_account",
		"bank_account": map[string]interface{}{
			"name":           accountHolderName,
			"ifsc":           ifsc,
			"account_number": accountNumber,
		},
	}
	body, err := r.client.FundAccount.Create(data, nil)
	if err != nil {
		return "", fmt.Errorf("razorpay create fund account failed: %v", err)
	}
	fundAccountID, ok := body["id"].(string)
	if !ok {
		return "", errors.New("failed to parse fund account id")
	}
	return fundAccountID, nil
}

func (r *razorpayClient) CreatePayout(accountNumber, fundAccountID string, amount float64, currency, purpose string, notes map[string]interface{}) (string, error) {
	data := map[string]interface{}{
		"account_number":       accountNumber,
		"fund_account_id":      fundAccountID,
		"amount":               int64(amount * 100),
		"currency":             currency,
		"mode":                 "IMPS",
		"purpose":              purpose,
		"queue_if_low_balance": true,
	}
	if notes != nil {
		data["notes"] = notes
	}
	// Payout resource doesn't have Create; use underlying request directly
	body, err := r.client.Post("/v1/payouts", data, nil)
	if err != nil {
		return "", fmt.Errorf("razorpay create payout failed: %v", err)
	}
	payoutID, ok := body["id"].(string)
	if !ok {
		return "", errors.New("failed to parse payout id")
	}
	return payoutID, nil
}

func (r *razorpayClient) CreateSubscription(planID string, totalCount int, customerNotify int, notes map[string]interface{}) (string, string, error) {
	data := map[string]interface{}{
		"plan_id":         planID,
		"total_count":     totalCount,
		"customer_notify": customerNotify,
	}
	if notes != nil {
		data["notes"] = notes
	}

	body, err := r.client.Subscription.Create(data, nil)
	if err != nil {
		return "", "", fmt.Errorf("razorpay create subscription failed: %v", err)
	}

	subscriptionID, ok := body["id"].(string)
	if !ok {
		return "", "", errors.New("failed to parse subscription id")
	}

	shortURL, _ := body["short_url"].(string)

	return subscriptionID, shortURL, nil
}
