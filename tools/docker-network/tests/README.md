# Docker Tests

These tests and the `DockerTestFramework` are using the Docker network and simulate a real network environment with multiple nodes.
They are therefore fully-fledged integration tests that test the entire node and inx-* connections.

The tests are by default excluded from compilation and tests with `//go:build dockertests`.
To run the tests, the `dockertests` build tag must be added when compiling or simply use the `run_tests.sh` script. 

## Running the tests
To run the tests, simply execute the following command:

```bash
# run all tests
./run_tests.sh

# or to run a specific tests
./run_tests.sh Test_Delegation Test_NoCandidacyPayload
```

## Run with script
The script builds and pulls the images, then runs the tests.

Available tests:
* SmallerCommittee
* ReuseDueToNoFinalization
* NoCandidacyPayload
* Staking
* Delegation

To run the tests, simply execute the following command:
```bash
# run all tests
./run_test.sh

# or to run a specific test 
./run_test.sh SmallerCommittee
```
