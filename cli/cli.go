package cli

import (
	"fmt"
	"io"

	"github.com/alecthomas/kong"
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
	PathToCSV string `required help:"the path the CSV file full of historical donor information."`
}

func (cmd *BackfillCmd) Run(env *Environment, client http.Client) error {
	account, err := client.FindAccount(cmd.AccountID)
	if err != nil {
		panic(err.Error())
	}

	fmt.Printf("Account information, %+v\n", account)

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
