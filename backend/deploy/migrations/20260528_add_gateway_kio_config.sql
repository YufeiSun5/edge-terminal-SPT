ALTER TABLE sys_gateways
  ADD COLUMN parser_type VARCHAR(64) DEFAULT 'kingiot_kio' AFTER qos,
  ADD COLUMN kio_client_id VARCHAR(128) DEFAULT '' AFTER parser_type,
  ADD COLUMN kio_writer VARCHAR(128) DEFAULT '' AFTER kio_client_id,
  ADD COLUMN kio_write_username VARCHAR(128) DEFAULT '' AFTER kio_writer,
  ADD COLUMN kio_write_password VARCHAR(255) DEFAULT '' AFTER kio_write_username,
  ADD COLUMN setdata_topic VARCHAR(255) DEFAULT '' AFTER kio_write_password,
  ADD COLUMN write_result_topic VARCHAR(255) DEFAULT '' AFTER setdata_topic,
  ADD COLUMN query_all_topic VARCHAR(255) DEFAULT '' AFTER write_result_topic;

UPDATE sys_gateways
SET
  name = 'default-kingiot-kio',
  broker = 'tcp://127.0.0.1:1883',
  client_id = 'edge-local-kio',
  username = 'Admin',
  password = 'admin',
  topic = 'datachange_S_KIO_Project',
  qos = 2,
  parser_type = 'kingiot_kio',
  kio_client_id = 'S_KIO_Project',
  kio_writer = 'edge-test',
  kio_write_username = 'sa',
  kio_write_password = 'C12E01F2A13FF5587E1E9E4AEDB8242D',
  setdata_topic = 'setdata_S_KIO_Project',
  write_result_topic = 'setdata_result_S_KIO_Project_edge-test',
  query_all_topic = 'Query_AllKIOTags_S_KIO_Project'
WHERE id = 1 OR topic = 'datachange_S_KIO_Project';
