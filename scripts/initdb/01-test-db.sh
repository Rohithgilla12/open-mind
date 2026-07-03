#!/bin/bash
set -e
psql -v ON_ERROR_STOP=1 -U openmind -d postgres -c "CREATE DATABASE openmind_test OWNER openmind;"
