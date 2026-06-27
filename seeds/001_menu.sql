INSERT INTO categories (id, name, sort_order)
VALUES
  ('1', 'Coffee', 1),
  ('2', 'Food', 2)
ON CONFLICT (id) DO NOTHING;

INSERT INTO menu_items (id, category_id, name, description, price_cents, currency, availability)
VALUES
  ('1', '1', 'Spanish Latte', 'Espresso with textured milk and condensed milk.', 950, 'MYR', 'in_stock'),
  ('2', '1', 'Americano', 'Espresso with hot water.', 700, 'MYR', 'in_stock'),
  ('3', '2', 'Butter Croissant', 'Flaky pastry with butter.', 850, 'MYR', 'in_stock')
ON CONFLICT (id) DO NOTHING;
