package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/willmadison/donately-sync-tools/donately"
	"github.com/willmadison/donately-sync-tools/donately/http"
)

// Environment provides an abstraction around the execution environment
type Environment struct {
	Stderr io.Writer
	Stdout io.Writer
	Stdin  io.Reader
}

type BackfillCmd struct {
	AccountID  string `required help:"the account id that this backfill should take place in."`
	PathToCSV  string `required help:"the absolute path to the CSV file full of historical donation information."`
	CampaignID string `required help:"the campaign id that this backfill should take place in."`
}

func (cmd *BackfillCmd) Run(env *Environment, client http.Client) error {
	account, err := client.FindAccount(cmd.AccountID)
	if err != nil {
		panic(err.Error())
	}

	fmt.Printf("Account information, %+v\n", account)

	campaign, err := client.FindCampaign(cmd.CampaignID, account)
	if err != nil {
		panic(err.Error())
	}

	fmt.Printf("Campaign information, %+v\n", campaign)

	in, err := os.Open(cmd.PathToCSV)
	if err != nil {
		panic(err)
	}

	var allDonors []donately.Person

	offset := 0
	limit := 100

	for {
		donors, err := client.ListPeople(account, offset, limit)
		if err != nil {
			return err
		}

		if len(donors) == 0 {
			break
		}

		allDonors = append(allDonors, donors...)
		offset += len(donors) + 1
	}

	donorsByEmailAddress := map[string]donately.Person{}
	donorsByPersonID := map[string]donately.Person{}

	for _, donor := range allDonors {
		donorsByEmailAddress[strings.ToLower(donor.Email)] = donor
		donorsByPersonID[donor.ID] = donor
	}

	var allDonations []donately.Donation

	offset = 0
	limit = 100

	for {
		donations, err := client.ListDonations(account, offset, limit)
		if err != nil {
			return err
		}

		if len(donations) == 0 {
			break
		}

		allDonations = append(allDonations, donations...)
		offset += len(donations) + 1
	}

	donationsByPersonId := map[string][]donately.Donation{}

	for _, donation := range allDonations {
		if _, present := donationsByPersonId[donation.Person.ID]; !present {
			donationsByPersonId[donation.Person.ID] = []donately.Donation{}
		}

		donationsByPersonId[donation.Person.ID] = append(donationsByPersonId[donation.Person.ID], donation)
	}

	collectionRecords, err := donately.ParseCollectionReportCSV(in)

	if err != nil {
		return err
	}

	recordsByFailureReason := map[string][]donately.CollectionReportRecord{}

	for _, c := range collectionRecords {
		if person, present := donorsByEmailAddress[strings.ToLower(c.EmailAddress)]; !present {
			fmt.Printf("%v %v is missing in Donately, adding them in...\n", c.FirstName, c.LastName)

			p := donately.Person{
				Accounts:  []donately.Account{account},
				FirstName: c.FirstName,
				LastName:  c.LastName,
				Email:     c.EmailAddress,
			}

			savedPerson, err := client.SavePerson(p)
			if err != nil {
				fmt.Printf("Encountered an error saving this person: %+v. Skipping...\n", p)

				if _, accountedFor := recordsByFailureReason[err.Error()]; !accountedFor {
					recordsByFailureReason[err.Error()] = []donately.CollectionReportRecord{}

				}
				recordsByFailureReason[err.Error()] = append(recordsByFailureReason[err.Error()], c)
				continue
			}

			fmt.Printf("%v %v saved (personId=%v)\n", c.FirstName, c.LastName, savedPerson.ID)
		} else {
			// See how much of a delta there is between their historical total donations and what the record says they've given
			donations := donationsByPersonId[person.ID]

			var cumulativeDonationInCents int64

			for _, donation := range donations {
				cumulativeDonationInCents += donation.AmountInCents
			}

			cumulativeDonation := cumulativeDonationInCents / 100
			balanceDue := c.AmountPledged - float64(cumulativeDonation)

			delta := c.AmountDonated - float64(cumulativeDonation)

			if balanceDue == 0 {
				fmt.Printf("According to Donately, %v %v has actually met their %v pledge with no remaining due.\n", person.FirstName, person.LastName, c.AmountPledged)
			} else if balanceDue > 0 {
				fmt.Printf("According to Donately, %v %v has actually given %v of their %v pledge with %.2f remaining due.\n", person.FirstName, person.LastName, cumulativeDonation, c.AmountPledged, balanceDue)
			}

			if delta > 0 {
				fmt.Printf("According to Chi Tau records, %v %v has given %v of their %v pledge leaving a delta of %.2f to be recorded in Donately.\n", person.FirstName, person.LastName, c.AmountDonated, c.AmountPledged, delta)

				donationToSave := donately.Donation{
					Account:       account,
					Person:        person,
					Campaign:      campaign,
					DonationType:  "cash",
					Status:        "processed",
					AmountInCents: int64(delta * 100),
				}

				fmt.Println("##############################################################################")
				fmt.Printf("Saving the following donation: person_id: %v, donation_type: %v, amount_in_cents: %v (%v, %v) \n", person.ID, donationToSave.DonationType, donationToSave.AmountInCents, person.FirstName, person.LastName)
				fmt.Println("##############################################################################")

				savedDonation, err := client.SaveDonation(donationToSave)
				if err != nil {
					recordsByFailureReason[err.Error()] = append(recordsByFailureReason[err.Error()], c)
					continue
				}

				fmt.Printf("%v %v $%v donation saved (donationId=%v)\n", c.FirstName, c.LastName, delta, savedDonation.ID)
			}
		}
	}

	if len(recordsByFailureReason) > 0 {
		fmt.Println("The following Persons couldn't be saved or couldn't have their donation records recorded for one reason or another:")

		for reason, records := range recordsByFailureReason {
			fmt.Printf("Reason: %v\n", reason)

			for _, record := range records {
				fmt.Println("##############################################################################")
				fmt.Printf("Record: %+v\n", record)
				fmt.Println("##############################################################################")
			}

			fmt.Println()
			fmt.Println()
			fmt.Println()
		}
	}

	return nil
}

type CLI struct {
	Backfill BackfillCmd `cmd help:"Backfills Donately donors based on a given account_id and csv file of donor data."`
}

func Run(env Environment) int {
	app := CLI{}

	client, err := http.NewDonatelyClient()
	if err != nil {
		panic(err.Error())
	}

	cntx := kong.Parse(&app,
		kong.Name("backfill"),
		kong.Description("donately utils"),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
		}),
	)
	cntx.BindTo(client, (*http.Client)(nil))

	err = cntx.Run(&env)
	cntx.FatalIfErrorf(err)

	return 0
}
