package models

import "time"

type Availability string

const (
	InStock    Availability = "in_stock"
	OutOfStock Availability = "out_of_stock"
)

type OrderStatus string

const (
	Received  OrderStatus = "received"
	Preparing OrderStatus = "preparing"
	Ready     OrderStatus = "ready"
	Completed OrderStatus = "completed"
)

type Category struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	SortOrder int        `json:"sortOrder"`
	Items     []MenuItem `json:"items"`
}

type MenuItem struct {
	ID           string       `json:"id"`
	CategoryID   string       `json:"categoryId"`
	Name         string       `json:"name"`
	Description  string       `json:"description"`
	PriceCents   int          `json:"priceCents"`
	Currency     string       `json:"currency"`
	Availability Availability `json:"availability"`
}

type Order struct {
	ID            string      `json:"id"`
	Status        OrderStatus `json:"status"`
	SubtotalCents int         `json:"subtotalCents"`
	TotalCents    int         `json:"totalCents"`
	Currency      string      `json:"currency"`
	CreatedAt     time.Time   `json:"createdAt"`
	UpdatedAt     time.Time   `json:"updatedAt"`
	Items         []OrderItem `json:"items"`
}

type OrderItem struct {
	ID             string `json:"id"`
	MenuItemID     string `json:"menuItemId,omitempty"`
	Name           string `json:"name"`
	UnitPriceCents int    `json:"unitPriceCents"`
	Quantity       int    `json:"quantity"`
	LineTotalCents int    `json:"lineTotalCents"`
}
