#!/bin/sh
# Runs once on first boot of the v2 local Postgres (docker-entrypoint-initdb.d).
# Applies the SAME scripts the VPS clusters use (deploy/local-v2/sql = copy of
# docs/migration-postgres/v2/sql, see deploy/local-v2/README.md "Sync").
# Dev-only passwords: dev_<role>. Never reuse these anywhere else.
set -eu
SQL=/v2/sql
P="psql -v ON_ERROR_STOP=1 --username $POSTGRES_USER"

$P -d postgres \
  -v iam_owner_pw=dev_iam_owner         -v iam_app_pw=dev_iam_app \
  -v finance_owner_pw=dev_finance_owner -v finance_app_pw=dev_finance_app \
  -v ppc_owner_pw=dev_ppc_owner         -v ppc_app_pw=dev_ppc_app \
  -v backup_pw=dev_backup_ro -v monitor_pw=dev_monitor -v report_pw=dev_report_ro \
  -v ilham_pw=dev_ilham -v indra_pw=dev_indra \
  -f $SQL/00-cluster-roles.sql
$P -d postgres -f $SQL/10-create-databases.sql
for s in iam finance ppc; do
  $P -d ${s}_db -v svc=$s -f $SQL/20-db-grants.sql
done
$P -d finance_db -f $SQL/25-finance-runtime.sql
# 30-restricted-iam.sql needs the IAM tables -> runs in `make v2-migrate` after the
# migrations (same order as the VPS: 20/25/30 after restore).
