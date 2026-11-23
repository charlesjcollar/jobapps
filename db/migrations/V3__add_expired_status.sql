-- EXPIRED: the posting came down before I applied.
ALTER TABLE jobs DROP CONSTRAINT jobs_status_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_status_check CHECK (status IN (
    'NEW',
    'INTERESTED',
    'APPLIED',
    'INTERVIEWING',
    'OFFER',
    'USER_REJECTED',
    'COMPANY_REJECTED',
    'EXPIRED'
));
