package wallapop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

// The two endpoints and the verb come from a captured browser request. They are variables
// so a change on Wallapop's side is a redeploy (WALLA_PATH_ITEMS, WALLA_PATH_REACTIVATE,
// WALLA_REACTIVATE_METHOD) and not a rebuild.
var (
	PathItems        = "/api/v3/user/items"
	PathReactivate   = "/api/v3/items/%s/reactivate"
	ReactivateMethod = "PUT"
	ItemsPageSize    = 100
)

var ErrNoItemsDecoded = errors.New("wallapop: the items response carried no recognisable list")

// HeaderNextPage carries the pagination cursor. The body's meta object comes back empty.
const HeaderNextPage = "X-NextPage"

type Price struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}

// Flag is how the API answers a yes/no: {"flag": true}, and the whole object is absent
// when the answer is no.
type Flag struct {
	Flag bool `json:"flag"`
}

type Item struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	CategoryID   string `json:"category_id"`
	Slug         string `json:"slug"`
	CreatedDate  int64  `json:"created_date"`
	ModifiedDate int64  `json:"modified_date"`
	Price        Price  `json:"price"`
	Expired      *Flag  `json:"expired"`
}

func (i Item) Modified() time.Time {
	if i.ModifiedDate == 0 {
		return time.Time{}
	}
	return time.UnixMilli(i.ModifiedDate)
}

// NeedsReactivation is what the catalogue page draws the pink button from.
func (i Item) NeedsReactivation() bool { return i.Expired != nil && i.Expired.Flag }

func (i Item) String() string {
	return fmt.Sprintf("%s (%.0f %s)", i.Title, i.Price.Amount, i.Price.Currency)
}

// MyItems lists the whole catalogue, following the cursor while there is one.
func (c *Client) MyItems(ctx context.Context) ([]Item, error) {
	var all []Item
	next := ""
	for page := 0; page < 50; page++ {
		query := url.Values{}
		query.Set("limit", strconv.Itoa(ItemsPageSize))
		if next != "" {
			query.Set("next_page", next)
		}

		var raw json.RawMessage
		header, err := c.do(ctx, "GET", PathItems, query, nil, &raw)
		if err != nil {
			return nil, err
		}
		items, err := decodeItems(raw)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)

		next = header.Get(HeaderNextPage)
		if next == "" || len(items) == 0 {
			break
		}
	}
	return all, nil
}

// decodeItems accepts the {"data": [...]} wrapper and a bare array.
func decodeItems(raw json.RawMessage) ([]Item, error) {
	var wrapped struct {
		Data []Item `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil && wrapped.Data != nil {
		return wrapped.Data, nil
	}
	var bare []Item
	if err := json.Unmarshal(raw, &bare); err == nil {
		return bare, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrNoItemsDecoded, snippet(raw))
}

// Reactivate presses the button. A clean call answers 204 with no body.
func (c *Client) Reactivate(ctx context.Context, itemID string) error {
	_, err := c.do(ctx, ReactivateMethod, fmt.Sprintf(PathReactivate, itemID), nil, nil, nil)
	return err
}
