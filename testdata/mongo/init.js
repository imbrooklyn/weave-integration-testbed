const databaseName = process.env.MONGO_INITDB_DATABASE || "weave_testbed";
const applicationUser = process.env.WEAVE_TESTBED_MONGO_USER || "weave";
const applicationPassword = process.env.WEAVE_TESTBED_MONGO_PASSWORD || "weave_demo_only";
const target = db.getSiblingDB(databaseName);
const fs = require("fs");

if (target.getUser(applicationUser) === null) {
  target.createUser({
    user: applicationUser,
    pwd: applicationPassword,
    roles: [{role: "readWrite", db: databaseName}]
  });
}

const records = EJSON.parse(
  fs.readFileSync("/docker-entrypoint-initdb.d/records.json", "utf8")
);
const regexRecords = EJSON.parse(
  fs.readFileSync("/docker-entrypoint-initdb.d/regex_records.json", "utf8")
);

target.semantic_records.deleteMany({});
target.semantic_records.insertMany(records);
target.semantic_records.createIndex(
  {nullable_number: 1},
  {name: "nullable_number_1"}
);

target.regex_probe_records.deleteMany({});
target.regex_probe_records.insertMany(regexRecords);
target.regex_probe_records.createIndex(
  {text_value: 1},
  {name: "text_value_1"}
);
