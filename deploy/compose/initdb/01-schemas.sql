-- Same layout as the CloudNativePG bootstrap in deploy/charts/shorty:
-- one database, one schema + one role per service, no cross-schema access.
CREATE ROLE links     LOGIN PASSWORD 'links-local-only';
CREATE ROLE analytics LOGIN PASSWORD 'analytics-local-only';
REVOKE ALL ON DATABASE shorty FROM PUBLIC;
GRANT CONNECT ON DATABASE shorty TO links, analytics;
REVOKE ALL ON SCHEMA public FROM PUBLIC;
CREATE SCHEMA links     AUTHORIZATION links;
CREATE SCHEMA analytics AUTHORIZATION analytics;
