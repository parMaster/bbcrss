CREATE TABLE IF NOT EXISTS news (
	id SERIAL PRIMARY KEY,
	title text NOT NULL,
	link text NOT NULL DEFAULT '' UNIQUE,
	description text NOT NULL DEFAULT '',
	image text NOT NULL DEFAULT ''
);