-- Filled in by scan plugins when the posting lists them; editable in the UI.
-- Free text because postings format these many ways ("$150k-$190k", "Remote (US)", ...).
ALTER TABLE jobs
    ADD COLUMN salary_range TEXT,
    ADD COLUMN location     TEXT;
