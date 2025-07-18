package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/willmadison/donately-sync-tools/donately"
)

type Client interface {
	FindAccount(string) (donately.Account, error)
	ListPeople(donately.Account) ([]donately.Person, error)
	FindPerson(string, donately.Account) (donately.Person, error)
	Me() (donately.Person, error)
	SavePerson(donately.Person) (donately.Person, error)
	ListDonations(donately.Account) ([]donately.Donation, error)
	ListMyDonations() ([]donately.Donation, error)
	FindDonation(string, donately.Account) (donately.Donation, error)
	SaveDonation(donately.Donation) (donately.Donation, error)
	RefundDonation(donately.Donation, string) error
	SendDonationReceipt(donately.Donation) error
	ListSubscriptions(donately.Account) ([]donately.Subscription, error)
	ListMySubscriptions() ([]donately.Subscription, error)
	FindSubscription(string, donately.Account) (donately.Subscription, error)
	SaveSubscription(donately.Subscription) (donately.Subscription, error)
	ListCampaigns(donately.Account) ([]donately.Campaign, error)
	FindCampaign(string, donately.Account) (donately.Campaign, error)
	SaveCampaign(donately.Campaign) (donately.Campaign, error)
	DeleteCampaign(donately.Campaign) error
}

type donatelyClient struct {
	APIKey  string
	BaseURL string
	client  *http.Client
}

type APIResponse struct {
	Data   json.RawMessage `json:"data"`
	Error  *APIError       `json:"error"`
	Status int             `json:"status"`
}

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func NewDonatelyClient() (Client, error) {
	apiKey := os.Getenv("DONATELY_API_KEY")

	if apiKey == "" {
		return &donatelyClient{}, errors.New("missing Donately API key")
	}

	return &donatelyClient{
		APIKey:  apiKey,
		BaseURL: "https://api.donately.com/v2",
		client:  &http.Client{},
	}, nil
}

func (c *donatelyClient) makeRequest(method, endpoint string, body any) (*APIResponse, error) {
	return c.makeRequestWithContentType(method, endpoint, body, "application/json")
}

func (c *donatelyClient) makeRequestWithContentType(method, endpoint string, body any, contentType string) (*APIResponse, error) {
	var reqBody io.Reader
	if body != nil {
		switch contentType {
		case "application/x-www-form-urlencoded":
			if formData, ok := body.(url.Values); ok {
				reqBody = strings.NewReader(formData.Encode())
			} else {
				return nil, fmt.Errorf("body must be url.Values for form-encoded requests")
			}
		default:
			jsonBody, err := json.Marshal(body)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal request body: %w", err)
			}
			reqBody = bytes.NewReader(jsonBody)
		}
	}

	req, err := http.NewRequest(method, c.BaseURL+endpoint, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	donatelyVersion := os.Getenv("DONATELY_API_VERSION")

	if donatelyVersion == "" {
		donatelyVersion = "2019-03-15"
	}

	req.Header.Set("Donately-Version", donatelyVersion)
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var apiResp APIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if apiResp.Error != nil {
		return nil, fmt.Errorf("API error: %s - %s", apiResp.Error.Code, apiResp.Error.Message)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP error: %d", resp.StatusCode)
	}

	return &apiResp, nil
}

func (c *donatelyClient) FindAccount(id string) (donately.Account, error) {
	endpoint := fmt.Sprintf("/accounts/%s", url.PathEscape(id))

	resp, err := c.makeRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return donately.Account{}, err
	}

	var account donately.Account
	if err := json.Unmarshal(resp.Data, &account); err != nil {
		return donately.Account{}, fmt.Errorf("failed to unmarshal account: %w", err)
	}

	return account, nil
}

func (c *donatelyClient) ListPeople(account donately.Account) ([]donately.Person, error) {
	params := url.Values{}
	params.Set("account_id", account.ID)

	resp, err := c.makeRequest(http.MethodGet, "/people?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}

	var people []donately.Person
	if err := json.Unmarshal(resp.Data, &people); err != nil {
		return nil, fmt.Errorf("failed to unmarshal people: %w", err)
	}

	return people, nil
}

func (c *donatelyClient) FindPerson(id string, account donately.Account) (donately.Person, error) {
	endpoint := fmt.Sprintf("/people/%s", url.PathEscape(id))

	params := url.Values{}
	params.Set("account_id", account.ID)

	resp, err := c.makeRequest(http.MethodGet, endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return donately.Person{}, err
	}

	var person donately.Person
	if err := json.Unmarshal(resp.Data, &person); err != nil {
		return donately.Person{}, fmt.Errorf("failed to unmarshal person: %w", err)
	}

	return person, nil
}

func (c *donatelyClient) Me() (donately.Person, error) {
	resp, err := c.makeRequest(http.MethodGet, "/me", nil)
	if err != nil {
		return donately.Person{}, err
	}

	var person donately.Person
	if err := json.Unmarshal(resp.Data, &person); err != nil {
		return donately.Person{}, fmt.Errorf("failed to unmarshal person: %w", err)
	}

	return person, nil
}

func (c *donatelyClient) SavePerson(person donately.Person) (donately.Person, error) {
	var endpoint string

	if person.ID == "" {
		endpoint = "/people"
	} else {
		endpoint = fmt.Sprintf("/people/%s", url.PathEscape(person.ID))
	}

	if len(person.Accounts) == 0 || person.Accounts[0].ID == "" {
		return donately.Person{}, errors.New("missing account information")
	}

	accountId := person.Accounts[0].ID

	formData := url.Values{}

	formData.Set("account_id", accountId)

	if person.FirstName != "" {
		formData.Set("first_name", person.FirstName)
	}
	if person.LastName != "" {
		formData.Set("last_name", person.LastName)
	}
	if person.Email != "" {
		formData.Set("email", person.Email)
	}
	if person.PhoneNumber != "" {
		formData.Set("phone_number", person.PhoneNumber)
	}
	if person.StreetAddress != "" {
		formData.Set("street_address", person.StreetAddress)
	}
	if person.StreetAddress2 != "" {
		formData.Set("street_address_2", person.StreetAddress2)
	}
	if person.City != "" {
		formData.Set("city", person.City)
	}
	if person.State != "" {
		formData.Set("state", person.State)
	}
	if person.ZipCode != "" {
		formData.Set("zip_code", person.ZipCode)
	}
	if person.Country != "" {
		formData.Set("country", person.Country)
	}

	resp, err := c.makeRequestWithContentType(http.MethodPost, endpoint, formData, "application/x-www-form-urlencoded")
	if err != nil {
		return donately.Person{}, err
	}

	var savedPerson donately.Person
	if err := json.Unmarshal(resp.Data, &savedPerson); err != nil {
		return donately.Person{}, fmt.Errorf("failed to unmarshal saved person: %w", err)
	}

	return savedPerson, nil
}

// Donations operations
func (c *donatelyClient) ListDonations(account donately.Account) ([]donately.Donation, error) {
	params := url.Values{}
	params.Set("account_id", account.ID)

	resp, err := c.makeRequest(http.MethodGet, "/donations?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}

	var donations []donately.Donation
	if err := json.Unmarshal(resp.Data, &donations); err != nil {
		return nil, fmt.Errorf("failed to unmarshal donations: %w", err)
	}

	return donations, nil
}

func (c *donatelyClient) ListMyDonations() ([]donately.Donation, error) {
	resp, err := c.makeRequest(http.MethodGet, "/me/donations", nil)
	if err != nil {
		return nil, err
	}

	var donations []donately.Donation
	if err := json.Unmarshal(resp.Data, &donations); err != nil {
		return nil, fmt.Errorf("failed to unmarshal my donations: %w", err)
	}

	return donations, nil
}

func (c *donatelyClient) FindDonation(id string, account donately.Account) (donately.Donation, error) {
	params := url.Values{}
	params.Set("account_id", account.ID)

	endpoint := fmt.Sprintf("/donations/%s", url.PathEscape(id))
	resp, err := c.makeRequest(http.MethodGet, endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return donately.Donation{}, err
	}

	var donation donately.Donation
	if err := json.Unmarshal(resp.Data, &donation); err != nil {
		return donately.Donation{}, fmt.Errorf("failed to unmarshal donation: %w", err)
	}

	return donation, nil
}

func (c *donatelyClient) SaveDonation(donation donately.Donation) (donately.Donation, error) {
	var endpoint string

	if donation.ID == "" {
		endpoint = "/donations"
	} else {
		endpoint = fmt.Sprintf("/donations/%s", url.PathEscape(donation.ID))
	}

	if donation.Account.ID == "" {
		return donately.Donation{}, errors.New("missing account information")
	}

	params := url.Values{}
	params.Set("account_id", donation.Account.ID)

	if donation.AmountInCents > 0 {
		params.Set("amount", fmt.Sprintf("%d", donation.AmountInCents))
	}
	if donation.Person.FirstName != "" {
		params.Set("first_name", donation.Person.FirstName)
	}
	if donation.Person.LastName != "" {
		params.Set("last_name", donation.Person.LastName)
	}
	if donation.Person.Email != "" {
		params.Set("email", donation.Person.Email)
	}
	if donation.Person.PhoneNumber != "" {
		params.Set("phone_number", donation.Person.PhoneNumber)
	}
	if donation.Comment != "" {
		params.Set("comment", donation.Comment)
	}
	if donation.Anonymous {
		params.Set("anonymous", "true")
	}
	if donation.OnBehalfOf != "" {
		params.Set("on_behalf_of", donation.OnBehalfOf)
	}
	if donation.Person.StreetAddress != "" {
		params.Set("street_address", donation.Person.StreetAddress)
	}
	if donation.MetaData.DonorPaysFees > 0 {
		params.Set("donor_pays_fees", "true")
	}

	if len(params) > 0 {
		endpoint += "?" + params.Encode()
	}

	resp, err := c.makeRequest(http.MethodPost, endpoint, nil)
	if err != nil {
		return donately.Donation{}, err
	}

	var savedDonation donately.Donation
	if err := json.Unmarshal(resp.Data, &savedDonation); err != nil {
		return donately.Donation{}, fmt.Errorf("failed to unmarshal saved donation: %w", err)
	}

	return savedDonation, nil
}

func (c *donatelyClient) RefundDonation(donation donately.Donation, reason string) error {
	endpoint := fmt.Sprintf("/donations/%s/refund", url.PathEscape(donation.ID))

	if donation.Account.ID == "" {
		return errors.New("missing account information")
	}

	formData := url.Values{}
	formData.Set("account_id", donation.Account.ID)
	formData.Set("refund_reason", reason)

	_, err := c.makeRequest(http.MethodPost, endpoint, formData)
	return err
}

func (c *donatelyClient) SendDonationReceipt(donation donately.Donation) error {
	endpoint := fmt.Sprintf("/donations/%s/receipt", url.PathEscape(donation.ID))
	_, err := c.makeRequest(http.MethodPost, endpoint, nil)
	return err
}

// Subscriptions operations
func (c *donatelyClient) ListSubscriptions(account donately.Account) ([]donately.Subscription, error) {
	params := url.Values{}
	params.Set("account_id", account.ID)

	resp, err := c.makeRequest(http.MethodGet, "/subscriptions?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}

	var subscriptions []donately.Subscription
	if err := json.Unmarshal(resp.Data, &subscriptions); err != nil {
		return nil, fmt.Errorf("failed to unmarshal subscriptions: %w", err)
	}

	return subscriptions, nil
}

func (c *donatelyClient) ListMySubscriptions() ([]donately.Subscription, error) {
	resp, err := c.makeRequest(http.MethodGet, "/me/subscriptions", nil)
	if err != nil {
		return nil, err
	}

	var subscriptions []donately.Subscription
	if err := json.Unmarshal(resp.Data, &subscriptions); err != nil {
		return nil, fmt.Errorf("failed to unmarshal my subscriptions: %w", err)
	}

	return subscriptions, nil
}

func (c *donatelyClient) FindSubscription(id string, account donately.Account) (donately.Subscription, error) {
	endpoint := fmt.Sprintf("/subscriptions/%s", url.PathEscape(id))

	params := url.Values{}
	params.Set("account_id", account.ID)

	resp, err := c.makeRequest(http.MethodGet, endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return donately.Subscription{}, err
	}

	var subscription donately.Subscription
	if err := json.Unmarshal(resp.Data, &subscription); err != nil {
		return donately.Subscription{}, fmt.Errorf("failed to unmarshal subscription: %w", err)
	}

	return subscription, nil
}

func (c *donatelyClient) SaveSubscription(subscription donately.Subscription) (donately.Subscription, error) {
	var endpoint string

	if subscription.ID == "" {
		endpoint = "/subscriptions"
	} else {
		endpoint = fmt.Sprintf("/subscriptions/%s", url.PathEscape(subscription.ID))
	}

	resp, err := c.makeRequest(http.MethodPost, endpoint, subscription)
	if err != nil {
		return donately.Subscription{}, err
	}

	var savedSubscription donately.Subscription
	if err := json.Unmarshal(resp.Data, &savedSubscription); err != nil {
		return donately.Subscription{}, fmt.Errorf("failed to unmarshal saved subscription: %w", err)
	}

	return savedSubscription, nil
}

// Campaigns operations
func (c *donatelyClient) ListCampaigns(account donately.Account) ([]donately.Campaign, error) {
	params := url.Values{}
	params.Set("account_id", account.ID)

	resp, err := c.makeRequest(http.MethodGet, "/campaigns?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}

	var campaigns []donately.Campaign
	if err := json.Unmarshal(resp.Data, &campaigns); err != nil {
		return nil, fmt.Errorf("failed to unmarshal campaigns: %w", err)
	}

	return campaigns, nil
}

func (c *donatelyClient) FindCampaign(id string, account donately.Account) (donately.Campaign, error) {
	endpoint := fmt.Sprintf("/campaigns/%s", url.PathEscape(id))

	params := url.Values{}
	params.Add("account_id", account.ID)

	resp, err := c.makeRequest(http.MethodGet, endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return donately.Campaign{}, err
	}

	var campaign donately.Campaign
	if err := json.Unmarshal(resp.Data, &campaign); err != nil {
		return donately.Campaign{}, fmt.Errorf("failed to unmarshal campaign: %w", err)
	}

	return campaign, nil
}

func (c *donatelyClient) SaveCampaign(campaign donately.Campaign) (donately.Campaign, error) {
	var endpoint string

	if campaign.ID == "" {
		endpoint = "/campaigns"
	} else {
		endpoint = fmt.Sprintf("/campaigns/%s", url.PathEscape(campaign.ID))
	}

	resp, err := c.makeRequest(http.MethodPost, endpoint, campaign)
	if err != nil {
		return donately.Campaign{}, err
	}

	var savedCampaign donately.Campaign
	if err := json.Unmarshal(resp.Data, &savedCampaign); err != nil {
		return donately.Campaign{}, fmt.Errorf("failed to unmarshal saved campaign: %w", err)
	}

	return savedCampaign, nil
}

func (c *donatelyClient) DeleteCampaign(campaign donately.Campaign) error {
	endpoint := fmt.Sprintf("/campaigns/%s", url.PathEscape(campaign.ID))
	_, err := c.makeRequest(http.MethodDelete, endpoint, nil)
	return err
}
