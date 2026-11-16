-- Optional location link shared by the host (Google Maps, Apple Maps, …).
ALTER TABLE events ADD COLUMN map_url text NOT NULL DEFAULT '';
