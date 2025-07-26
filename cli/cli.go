package cli

import (
	"context"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/gin-gonic/gin"
	"github.com/willmadison/donately-sync-tools/donately"
	donatelyhttp "github.com/willmadison/donately-sync-tools/donately/http"
)

const epsilon = 1e-9

// Environment provides an abstraction around the execution environment
type Environment struct {
	Stderr io.Writer
	Stdout io.Writer
	Stdin  io.Reader
	Files  embed.FS
	UI     embed.FS
}

type BackfillCmd struct {
	AccountID  string `required help:"the account id that this backfill should take place in."`
	CampaignID string `required help:"the campaign id that this backfill should take place in."`
}

func (cmd *BackfillCmd) Run(env *Environment, client donatelyhttp.Client, adjustmentStore donately.AdjustmentStore) error {
	account, err := client.FindAccount(cmd.AccountID)
	if err != nil {
		panic(err.Error())
	}

	campaign, err := client.FindCampaign(cmd.CampaignID, account)
	if err != nil {
		panic(err.Error())
	}

	in, err := env.Files.Open("static/inputs/records.csv")
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

			// Handle any donation adjustments (i.e. program/fundraisers this brother may have participated in)

			adjustments, err := adjustmentStore.GetAdustmentsByPerson(context.Background(), person)
			if err == nil && len(adjustments) != len(c.Adjustments) {
				fmt.Printf("there's an adjustment discrepancy for %v %v let's update our data based on the official record.\n", c.FirstName, c.LastName)
				err := adjustmentStore.SaveAdjustments(context.Background(), person, c.Adjustments)
				if err != nil {
					fmt.Printf("encounterd an error processing adjustments for %v %v, will retry later. (%v)\n", c.FirstName, c.LastName, err.Error())
				}
			} else if err != nil {
				fmt.Printf("encounterd an error processing adjustments for %v %v, skipping that step for now (%v).\n", c.FirstName, c.LastName, err.Error())
			}

			var cumulativeDonationInCents int64

			for _, donation := range donations {
				cumulativeDonationInCents += donation.AmountInCents
			}

			cumulativeDonation := cumulativeDonationInCents / 100
			expectedBalanceDue := c.AmountPledged - float64(cumulativeDonation)

			if expectedBalanceDue-c.AmountDue < epsilon || c.AmountDue == 0 {
				fmt.Printf("According to Donately and/or our records regarding adjustments and other programs, %v %v has actually met their %v pledge with no remaining due.\n", person.FirstName, person.LastName, c.AmountPledged)
				continue
			}

			fmt.Printf("According to Donately, %v %v has actually given %v of their %v pledge with %.2f remaining due (accounting for adjustments and other fundraising programs).\n", person.FirstName, person.LastName, cumulativeDonation, c.AmountPledged, c.AmountDue)

			delta := c.AmountDonated - float64(cumulativeDonation)

			if delta > 0 && delta >= .5 {
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

type ServeCmd struct {
	AccountID  string `required help:"the account id that this service should leverage."`
	CampaignID string `required help:"the campaign id that this service should leverage"`
}

func (cmd *ServeCmd) Run(env *Environment, client donatelyhttp.Client, adjustmentStore donately.AdjustmentStore) error {
	account, err := client.FindAccount(cmd.AccountID)
	if err != nil {
		panic(err.Error())
	}

	campaign, err := client.FindCampaign(cmd.CampaignID, account)
	if err != nil {
		panic(err.Error())
	}

	in, err := env.Files.Open("static/inputs/records.csv")
	if err != nil {
		panic(err)
	}

	collectionRecords, err := donately.ParseCollectionReportCSV(in)

	if err != nil {
		panic(err)
	}

	r := gin.Default()

	api := r.Group("/api")
	{
		api.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})

		api.GET("/campaign/overview", donatelyhttp.CampaignOverviewHandler(client, adjustmentStore, account, campaign, collectionRecords))
	}

	uiFS, err := fs.Sub(env.UI, "static/donor-dashboard/dist")
	if err != nil {
		panic(err)
	}

	r.NoRoute(gin.WrapH(http.FileServer(http.FS(uiFS))))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen error: %s\n", err)
		}
	}()
	log.Printf("Server running on :%v\n", port)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %s", err)
	}

	log.Println("Server exited gracefully")

	return nil
}

type CLI struct {
	Backfill BackfillCmd `cmd help:"Backfills Donately donors based on a given account_id and csv file of donor data."`
	Serve    ServeCmd    `cmd help:"Serves our campaign progress service/ui for visualizing how brothers have progressed on their pledges."`
}

func Run(env Environment) int {
	app := CLI{}

	client, err := donatelyhttp.NewDonatelyClient()
	if err != nil {
		panic(err.Error())
	}

	adjustmentStore, err := donately.NewAdjustmentStore()
	if err != nil {
		panic(err.Error())
	}

	cntx := kong.Parse(&app,
		kong.Description("donately utils"),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
		}),
	)

	cntx.BindTo(client, (*donatelyhttp.Client)(nil))
	cntx.BindTo(adjustmentStore, (*donately.AdjustmentStore)(nil))

	err = cntx.Run(&env)
	cntx.FatalIfErrorf(err)

	return 0
}
