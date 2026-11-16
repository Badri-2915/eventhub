-- price_paise stays what the attendee pays; list price + discount are kept so the
-- host can edit them and the site can show "₹500 → ₹400 (20% off)". 0 means free.
ALTER TABLE ticket_categories
    ADD COLUMN list_price_paise int NOT NULL DEFAULT 0 CHECK (list_price_paise >= 0),
    ADD COLUMN discount_percent int NOT NULL DEFAULT 0 CHECK (discount_percent BETWEEN 0 AND 90);

UPDATE ticket_categories SET list_price_paise = price_paise;
