-- Job postings tracked by jobapps.

CREATE TABLE jobs (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    company_name    TEXT        NOT NULL,
    job_title       TEXT        NOT NULL,
    posting_id      TEXT        NOT NULL,  -- the company's own ID for the posting
    link            TEXT        NOT NULL,
    posted_date     DATE,                  -- null when the source doesn't say
    retrieved_date  TIMESTAMPTZ NOT NULL DEFAULT now(),
    status          TEXT        NOT NULL DEFAULT 'NEW',
    last_updated    TIMESTAMPTZ NOT NULL DEFAULT now(),
    notes           TEXT,

    CONSTRAINT jobs_status_check CHECK (status IN (
        'NEW',               -- found, not yet reviewed
        'INTERESTED',        -- reviewed, plan to apply
        'APPLIED',
        'INTERVIEWING',
        'OFFER',
        'USER_REJECTED',     -- I decided not to pursue it
        'COMPANY_REJECTED'   -- the company turned me down
    )),

    -- Lets the engine skip postings it has already stored.
    CONSTRAINT jobs_company_posting_unique UNIQUE (company_name, posting_id)
);

CREATE INDEX jobs_status_idx ON jobs (status);

-- Keep last_updated current on every UPDATE.
CREATE FUNCTION set_last_updated() RETURNS TRIGGER AS $$
BEGIN
    NEW.last_updated = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER jobs_set_last_updated
    BEFORE UPDATE ON jobs
    FOR EACH ROW
    EXECUTE FUNCTION set_last_updated();
