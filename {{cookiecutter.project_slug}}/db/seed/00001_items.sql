-- Example data for local development (task db:seed). Idempotent.
INSERT INTO items (id, name, description) VALUES
    ('01900000-0000-7000-8000-000000000001', 'first', 'seeded example'),
    ('01900000-0000-7000-8000-000000000002', 'second', 'seeded example')
ON CONFLICT (id) DO NOTHING;
