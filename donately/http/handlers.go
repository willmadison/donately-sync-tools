package http

import (
	"context"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/willmadison/donately-sync-tools/donately"
)

func CampaignOverviewHandler(client Client, adjustmentStore donately.AdjustmentStore, account donately.Account, campaign donately.Campaign, collectionRecords []donately.CollectionReportRecord) func(*gin.Context) {
	return func(c *gin.Context) {
		var everyone []donately.Person

		offset := 0
		limit := 100

		for {
			donors, err := client.ListPeople(account, offset, limit)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"error":   "internal server error",
					"details": err.Error(),
				})
				return
			}

			if len(donors) == 0 {
				break
			}

			everyone = append(everyone, donors...)
			offset += len(donors) + 1
		}

		peopleById := map[string]donately.Person{}

		for _, person := range everyone {
			peopleById[person.ID] = person
		}

		pledgeAmountByEmail := map[string]float64{}

		for _, collectionRecord := range collectionRecords {
			pledgeAmountByEmail[strings.ToLower(collectionRecord.EmailAddress)] = collectionRecord.AmountPledged
		}

		var allDonations []donately.Donation

		offset = 0
		limit = 100

		for {
			donations, err := client.ListDonations(account, offset, limit)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"error":   "internal server error",
					"details": err.Error(),
				})
				return
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

		var donors []donately.Donor

		for _, person := range everyone {
			if person.FirstName == "Testy" {
				continue
			}

			donations := donationsByPersonId[person.ID]

			if donations == nil {
				donations = []donately.Donation{}
			}

			adjustments, err := adjustmentStore.GetAdustmentsByPerson(context.Background(), person)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"error":   "internal server error",
					"details": err.Error(),
				})
				return
			}

			if adjustments == nil {
				adjustments = []donately.Adjustment{}
			}

			pledge := pledgeAmountByEmail[strings.ToLower(person.Email)]

			if pledge == 0 {
				continue
			}

			donors = append(donors, donately.Donor{
				Person:      person,
				Adjustments: adjustments,
				Donations:   donations,
				Pledge:      pledge,
			})
		}

		sort.Slice(donors, func(i, j int) bool {
			return donors[i].Person.LastName < donors[j].Person.LastName
		})

		overview := donately.CampaignOverview{
			ID:                  campaign.ID,
			Title:               campaign.Title,
			Slug:                campaign.Slug,
			Type:                campaign.Type,
			URL:                 campaign.URL,
			Status:              campaign.Status,
			Permalink:           campaign.Permalink,
			Description:         campaign.Description,
			Content:             campaign.Content,
			Created:             campaign.Created,
			Updated:             campaign.Updated,
			StartDate:           campaign.StartDate,
			EndDate:             campaign.EndDate,
			GoalInCents:         campaign.GoalInCents,
			AmountRaisedInCents: campaign.AmountRaisedInCents,
			PercentFunded:       campaign.PercentFunded,
			Donors:              donors,
		}

		c.JSON(http.StatusOK, overview)
	}
}
