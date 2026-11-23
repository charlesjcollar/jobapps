-- Recruiter / hiring manager contact, filled in once the company reaches out.
ALTER TABLE jobs
    ADD COLUMN contact_name  TEXT,
    ADD COLUMN contact_email TEXT,
    ADD COLUMN contact_phone TEXT;
