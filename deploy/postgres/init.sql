-- Runs once when the dev database volume is first created.
-- `callcenter` (owner) runs migrations; `callcenter_app` is the runtime role.
-- The runtime role is deliberately NOT the table owner and has no BYPASSRLS,
-- so row-level security policies always apply to the API.
CREATE ROLE callcenter_app LOGIN PASSWORD 'callcenter_app';

CREATE DATABASE callcenter_test OWNER callcenter;

GRANT CONNECT ON DATABASE callcenter TO callcenter_app;
GRANT CONNECT ON DATABASE callcenter_test TO callcenter_app;
