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
	AccountID string `required help:"the account id that this backfill should take place in."`
	PathToCSV string `required help:"the absolute path to the CSV file full of historical donation information."`
}

func (cmd *BackfillCmd) Run(env *Environment, client http.Client) error {
	account, err := client.FindAccount(cmd.AccountID)
	if err != nil {
		panic(err.Error())
	}

	fmt.Printf("Account information, %+v\n", account)

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

	for _, donor := range allDonors {
		donorsByEmailAddress[strings.ToLower(donor.Email)] = donor
	}

	collectionRecords, err := donately.ParseCollectionReportCSV(in)

	if err != nil {
		return err
	}

	recordsByFailureReason := map[string][]donately.CollectionReportRecord{}

	for _, c := range collectionRecords {
		if _, present := donorsByEmailAddress[strings.ToLower(c.EmailAddress)]; !present {
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
		}
	}

	if len(recordsByFailureReason) > 0 {
		fmt.Println("The following Persons couldn't be saved for one reason or another:")

		for reason, records := range recordsByFailureReason {
			fmt.Printf("Reason: %v\n", reason)

			for _, record := range records {
				fmt.Printf("Record: %v\n", record)
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
