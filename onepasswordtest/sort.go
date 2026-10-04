package onepasswordtest

import (
	"sort"

	"github.com/1password/onepassword-sdk-go"
)

func sortOverviews(o []onepassword.ItemOverview) {
	sort.Slice(o, func(i, j int) bool { return o[i].ID < o[j].ID })
}

func sortVaults(v []onepassword.VaultOverview) {
	sort.Slice(v, func(i, j int) bool { return v[i].ID < v[j].ID })
}
