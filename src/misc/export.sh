(cat create_tables.sql export.sql) |duckdb   -json|jq .[].json_result -r|jq . > export.json