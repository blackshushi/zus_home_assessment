package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/alex/zus_home_assessment/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound           = errors.New("not found")
	ErrItemUnavailable    = errors.New("menu item unavailable")
	ErrInvalidTransition  = errors.New("invalid status transition")
	ErrMixedCurrencyOrder = errors.New("order contains mixed currencies")
)

type Store struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

type dbtx interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func (s *Store) GetMenu(ctx context.Context) ([]models.Category, error) {
	rows, err := s.db.Query(ctx, `
		SELECT c.id, c.name, c.sort_order,
		       i.id, i.category_id, i.name, i.description, i.price_cents, i.currency, i.availability
		FROM categories c
		LEFT JOIN menu_items i ON i.category_id = c.id AND i.active = true
		WHERE c.active = true
		ORDER BY c.sort_order, c.name, i.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byID := map[string]*models.Category{}
	order := []string{}

	for rows.Next() {
		var category models.Category
		var itemID, itemCategoryID, itemName, itemDescription, itemCurrency *string
		var itemPriceCents *int
		var itemAvailability *models.Availability
		if err := rows.Scan(
			&category.ID,
			&category.Name,
			&category.SortOrder,
			&itemID,
			&itemCategoryID,
			&itemName,
			&itemDescription,
			&itemPriceCents,
			&itemCurrency,
			&itemAvailability,
		); err != nil {
			return nil, err
		}

		existing := byID[category.ID]
		if existing == nil {
			category.Items = []models.MenuItem{}
			byID[category.ID] = &category
			order = append(order, category.ID)
			existing = &category
		}

		if itemID != nil {
			item := models.MenuItem{
				ID:           *itemID,
				CategoryID:   *itemCategoryID,
				Name:         *itemName,
				Description:  *itemDescription,
				PriceCents:   *itemPriceCents,
				Currency:     *itemCurrency,
				Availability: *itemAvailability,
			}
			existing.Items = append(existing.Items, item)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	categories := make([]models.Category, 0, len(order))
	for _, id := range order {
		categories = append(categories, *byID[id])
	}
	return categories, nil
}

func (s *Store) GetMenuItem(ctx context.Context, id string) (models.MenuItem, error) {
	return scanMenuItem(s.db.QueryRow(ctx, `
		SELECT id, category_id, name, description, price_cents, currency, availability
		FROM menu_items
		WHERE id = $1 AND active = true`, id))
}

func (s *Store) UpdateMenuItemAvailability(ctx context.Context, id string, availability models.Availability) (models.MenuItem, error) {
	return scanMenuItem(s.db.QueryRow(ctx, `
		UPDATE menu_items
		SET availability = $2, updated_at = now()
		WHERE id = $1 AND active = true
		RETURNING id, category_id, name, description, price_cents, currency, availability`, id, availability))
}

type OrderRequestItem struct {
	MenuItemID string
	Quantity   int
}

func (s *Store) CreateOrder(ctx context.Context, requestedItems []OrderRequestItem) (models.Order, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return models.Order{}, err
	}
	defer tx.Rollback(ctx)

	subtotal := 0
	currency := ""
	orderItems := make([]models.OrderItem, 0, len(requestedItems))

	for _, requested := range requestedItems {
		item, err := scanMenuItem(tx.QueryRow(ctx, `
			SELECT id, category_id, name, description, price_cents, currency, availability
			FROM menu_items
			WHERE id = $1 AND active = true
			FOR UPDATE`, requested.MenuItemID))
		if err != nil {
			return models.Order{}, err
		}
		if item.Availability != models.InStock {
			return models.Order{}, fmt.Errorf("%w: %s", ErrItemUnavailable, requested.MenuItemID)
		}
		if currency == "" {
			currency = item.Currency
		}
		if currency != item.Currency {
			return models.Order{}, ErrMixedCurrencyOrder
		}

		lineTotal := item.PriceCents * requested.Quantity
		subtotal += lineTotal
		orderItems = append(orderItems, models.OrderItem{
			MenuItemID:     item.ID,
			Name:           item.Name,
			UnitPriceCents: item.PriceCents,
			Quantity:       requested.Quantity,
			LineTotalCents: lineTotal,
		})
	}

	order, err := scanOrder(tx.QueryRow(ctx, `
		INSERT INTO orders (status, subtotal_cents, total_cents, currency)
		VALUES ($1, $2, $3, $4)
		RETURNING id, status, subtotal_cents, total_cents, currency, created_at, updated_at`,
		models.Received, subtotal, subtotal, currency))
	if err != nil {
		return models.Order{}, err
	}

	for i, item := range orderItems {
		err := tx.QueryRow(ctx, `
			INSERT INTO order_items (order_id, menu_item_id, name, unit_price_cents, quantity, line_total_cents)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id`,
			order.ID, item.MenuItemID, item.Name, item.UnitPriceCents, item.Quantity, item.LineTotalCents,
		).Scan(&orderItems[i].ID)
		if err != nil {
			return models.Order{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return models.Order{}, err
	}

	order.Items = orderItems
	return order, nil
}

func (s *Store) GetOrder(ctx context.Context, id string) (models.Order, error) {
	order, err := scanOrder(s.db.QueryRow(ctx, `
		SELECT id, status, subtotal_cents, total_cents, currency, created_at, updated_at
		FROM orders
		WHERE id = $1`, id))
	if err != nil {
		return models.Order{}, err
	}

	items, err := getOrderItems(ctx, s.db, id)
	if err != nil {
		return models.Order{}, err
	}
	order.Items = items
	return order, nil
}

func (s *Store) UpdateOrderStatus(ctx context.Context, id string, next models.OrderStatus) (models.Order, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return models.Order{}, err
	}
	defer tx.Rollback(ctx)

	var current models.OrderStatus
	err = tx.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1 FOR UPDATE`, id).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Order{}, ErrNotFound
	}
	if err != nil {
		return models.Order{}, err
	}
	if !validTransition(current, next) {
		return models.Order{}, fmt.Errorf("%w: %s to %s", ErrInvalidTransition, current, next)
	}

	order, err := scanOrder(tx.QueryRow(ctx, `
		UPDATE orders
		SET status = $2, updated_at = now()
		WHERE id = $1
		RETURNING id, status, subtotal_cents, total_cents, currency, created_at, updated_at`, id, next))
	if err != nil {
		return models.Order{}, err
	}
	order.Items, err = getOrderItems(ctx, tx, id)
	if err != nil {
		return models.Order{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return models.Order{}, err
	}
	return order, nil
}

func scanMenuItem(row pgx.Row) (models.MenuItem, error) {
	var item models.MenuItem
	err := row.Scan(&item.ID, &item.CategoryID, &item.Name, &item.Description, &item.PriceCents, &item.Currency, &item.Availability)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.MenuItem{}, ErrNotFound
	}
	return item, err
}

func scanOrder(row pgx.Row) (models.Order, error) {
	var order models.Order
	err := row.Scan(&order.ID, &order.Status, &order.SubtotalCents, &order.TotalCents, &order.Currency, &order.CreatedAt, &order.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Order{}, ErrNotFound
	}
	return order, err
}

func getOrderItems(ctx context.Context, q dbtx, orderID string) ([]models.OrderItem, error) {
	rows, err := q.Query(ctx, `
		SELECT id, COALESCE(menu_item_id::text, ''), name, unit_price_cents, quantity, line_total_cents
		FROM order_items
		WHERE order_id = $1
		ORDER BY created_at`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []models.OrderItem{}
	for rows.Next() {
		var item models.OrderItem
		if err := rows.Scan(&item.ID, &item.MenuItemID, &item.Name, &item.UnitPriceCents, &item.Quantity, &item.LineTotalCents); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func validTransition(current, next models.OrderStatus) bool {
	transitions := map[models.OrderStatus]models.OrderStatus{
		models.Received:  models.Preparing,
		models.Preparing: models.Ready,
		models.Ready:     models.Completed,
	}
	return transitions[current] == next
}
