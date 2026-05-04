#!/bin/bash
set -e

SA_PASSWORD="${SA_PASSWORD:=HixTest_Pass1!}"

echo "Starting SQL Server..."
/opt/mssql/bin/sqlservr &
SQLSERVER_PID=$!

echo "Waiting for SQL Server to be ready..."
for i in {1..60}; do
    /opt/mssql-tools18/bin/sqlcmd -S localhost -U sa -P "$SA_PASSWORD" -Q "SELECT 1" -C &>/dev/null && {
        echo "SQL Server is ready!"
        break
    }
    if [ $i -eq 60 ]; then
        echo "SQL Server did not start in time"
        kill $SQLSERVER_PID || true
        exit 1
    fi
    echo "Attempt $i/60..."
    sleep 2
done

echo "Running init script..."
/opt/mssql-tools18/bin/sqlcmd -S localhost -U sa -P "$SA_PASSWORD" -i /var/opt/mssql/init.sql -C
echo "Setup complete! SQL Server is running..."
wait $SQLSERVER_PID
