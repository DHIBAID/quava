package commands

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"quava-cli/models"
	"quava-cli/utils"

	"github.com/spf13/cobra"
)

// NotificationFilters edits which Android notifications the host displays.
var NotificationFilters = &cobra.Command{
	Use:   "filters <action> [value]",
	Short: "Choose which Android notifications are shown on the host.",
	Long: `View or edit Android-notification filters. Changes apply immediately and are saved.

Actions:
  show                        print the current filters
  mute-app <app>              hide notifications from an app (by Android app name)
  unmute-app <app>            stop hiding an app
  only-app <app>              show only the listed apps (repeat to add more)
  unonly-app <app>            remove an app from the only-list
  mute-category <category>    hide a category (e.g. promo, social, recommendation)
  unmute-category <category>  stop hiding a category
  silent <drop|preserve|normal>
                              how the host shows silent notifications the phone sends
                              (the phone's own setting decides what it sends)
  clear                       remove all filters`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(_ *cobra.Command, args []string) error { return runFilters(args) },
}

func fetchFilters() (models.NotificationFilters, error) {
	res, err := utils.Call(models.Request{Command: "notif-filters-get"})
	if err != nil {
		return models.NotificationFilters{}, err
	}
	if res.Filters == nil {
		return models.NotificationFilters{}, nil
	}
	return *res.Filters, nil
}

func runFilters(args []string) error {
	action := args[0]
	filters, err := fetchFilters()
	if err != nil {
		return err
	}

	if action == "show" {
		printFilters(filters)
		return nil
	}

	needValue := action != "clear"
	if needValue && len(args) != 2 || !needValue && len(args) != 1 {
		return fmt.Errorf("usage: quava notification filters %s%s", action, map[bool]string{true: " <value>", false: ""}[needValue])
	}
	value := ""
	if needValue {
		value = strings.TrimSpace(args[1])
		if value == "" {
			return errors.New("value must not be empty")
		}
	}

	switch action {
	case "mute-app":
		filters.MutedApps = addFold(filters.MutedApps, value)
	case "unmute-app":
		filters.MutedApps = removeFold(filters.MutedApps, value)
	case "only-app":
		filters.OnlyApps = addFold(filters.OnlyApps, value)
	case "unonly-app":
		filters.OnlyApps = removeFold(filters.OnlyApps, value)
	case "mute-category":
		filters.MutedCategories = addFold(filters.MutedCategories, value)
	case "unmute-category":
		filters.MutedCategories = removeFold(filters.MutedCategories, value)
	case "silent":
		filters.Silent = strings.ToLower(value)
	case "clear":
		filters = models.NotificationFilters{}
	default:
		return fmt.Errorf("unknown filters action %q (use show, mute-app, unmute-app, only-app, unonly-app, mute-category, unmute-category, silent, or clear)", action)
	}

	res, err := utils.Call(models.Request{Command: "notif-filters-set", Filters: &filters})
	if err != nil {
		return err
	}
	if res.Filters != nil {
		printFilters(*res.Filters)
	}
	return nil
}

func addFold(list []string, value string) []string {
	if slices.ContainsFunc(list, func(item string) bool { return strings.EqualFold(item, value) }) {
		return list
	}
	return append(list, value)
}

func removeFold(list []string, value string) []string {
	return slices.DeleteFunc(slices.Clone(list), func(item string) bool { return strings.EqualFold(item, value) })
}

func printFilters(f models.NotificationFilters) {
	show := func(items []string) string {
		if len(items) == 0 {
			return "(none)"
		}
		return strings.Join(items, ", ")
	}
	silent := f.Silent
	if silent == "" {
		silent = "preserve (default)"
	}
	fmt.Printf("muted apps:       %s\nonly apps:        %s\nmuted categories: %s\nsilent policy:    %s\n",
		show(f.MutedApps), show(f.OnlyApps), show(f.MutedCategories), silent)
}
