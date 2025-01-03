# FHIR Endpoint Project - Linux Usage Instructions

## Prerequisites
- Linux operating system
- The `fenix_endpoint_linux.tar.gz` file

## Setup and Usage

1. Extract the compressed archive:
   ```
   tar -xzvf fenix_endpoint_linux.tar.gz
   ```

2. Navigate to the extracted directory:
   ```
   cd fenix_endpoint_linux
   ```

3. Ensure the executable has the right permissions:
   ```
   chmod +x fenix_endpoint_test
   ```

4. Run the application:
   ```
   ./fenix_endpoint_test
   ```

The server should now be running on `http://localhost:8081`.

## Testing the Endpoints

You can use the provided `test_endpoint.http` file with VS Code's REST Client extension to test the endpoints easily.

Alternatively, use curl commands:

1. Get all current ICU patients:
   ```
   curl "http://localhost:8081/Encounter?location.type=ICU&status=in-progress"
   ```

2. Get observations for patient P001:
   ```
   curl "http://localhost:8081/Observation?patient=P001"
   ```

3. Get observations for patient P002:
   ```
   curl "http://localhost:8081/Observation?patient=P002"
   ```

4. Get observations for patient P003:
   ```
   curl "http://localhost:8081/Observation?patient=P003"
   ```

Check the `output/` directory for saved JSON responses and the `requests.log` file for a log of incoming requests.

## Note
The `input/` directory contains example FHIR JSON files. You can modify these or add new ones as needed for your testing purposes.


# FHIR Endpoint Project - Windows Usage Instructions

## Prerequisites
- Windows operating system
- The `fenix_endpoint_windows.zip` file

## Setup and Usage

1. Extract the zip file using Windows Explorer or any zip utility.

2. Open a Command Prompt and navigate to the extracted directory:
   ```
   cd path\to\extracted\fenix_endpoint_windows
   ```

3. Run the application:
   ```
   fenix_endpoint.exe
   ```

The server should now be running on `http://localhost:8081`.

## Testing the Endpoints

You can use the provided `test_endpoint.http` file with VS Code's REST Client extension to test the endpoints easily.

Alternatively, use curl commands in Command Prompt:

1. Get all current ICU patients:
   ```
   curl "http://localhost:8081/Encounter?location.type=ICU&status=in-progress"
   ```

2. Get observations for patient P001:
   ```
   curl "http://localhost:8081/Observation?patient=P001"
   ```

3. Get observations for patient P002:
   ```
   curl "http://localhost:8081/Observation?patient=P002"
   ```

4. Get observations for patient P003:
   ```
   curl "http://localhost:8081/Observation?patient=P003"
   ```

Check the `output\` directory for saved JSON responses and the `requests.log` file for a log of incoming requests.

## Note
The `input\` directory contains example FHIR JSON files. You can modify these or add new ones as needed for your testing purposes.