docker exec -it mongo mongosh --eval "
db = db.getSiblingDB('open5gs');
db.subscribers.deleteMany({});
print('All subscribers deleted');

// ─── Helper: suscriptor 4G (S1AP/Diameter) ───
function make4GSubscriber(imsi, imeisv) {
  return {
    imsi: imsi,
    msisdn: [],
    imeisv: imeisv,
    mme_host: 'mme.epc.mnc001.mcc001.3gppnetwork.org',
    mme_realm: 'epc.mnc001.mcc001.3gppnetwork.org',
    purge_flag: false,
    access_restriction_data: 32,
    subscriber_status: 0,
    operator_determined_barring: 0,
    network_access_mode: 0,
    subscribed_rau_tau_timer: 12,
    ambr: { downlink: { value: 1, unit: 3 }, uplink: { value: 1, unit: 3 } },
    schema_version: 1,
    security: {
      k: '8baf473f2f8fd09487cccbd7097c6862',
      amf: '8000',
      op: '11111111111111111111111111111111',
      opc: null,
      sqn: NumberLong('0')
    },
    slice: [{
      sst: 1,
      default_indicator: true,
      session: [{
        name: 'internet',
        type: 3,
        qos: {
          index: 9,
          arp: { priority_level: 8, pre_emption_capability: 1, pre_emption_vulnerability: 1 }
        },
        ambr: { downlink: { value: 1, unit: 3 }, uplink: { value: 1, unit: 3 } },
        pcc_rule: []
      }]
    }],
    __v: 0
  };
}

// ─── Helper: suscriptor 5G (NGAP/SBI) ───
function make5GSubscriber(imsi, imeisv, sst) {
  return {
    imsi: imsi,
    msisdn: [],
    imeisv: imeisv,
    mme_host: [],
    mme_realm: [],
    purge_flag: [],
    access_restriction_data: 32,
    subscriber_status: 0,
    operator_determined_barring: 0,
    network_access_mode: 0,
    subscribed_rau_tau_timer: 12,
    ambr: { downlink: { value: 1, unit: 3 }, uplink: { value: 1, unit: 3 } },
    schema_version: 1,
    security: {
      k: '8baf473f2f8fd09487cccbd7097c6862',
      amf: '8000',
      op: '11111111111111111111111111111111',
      opc: null,
      sqn: NumberLong('0')
    },
    slice: [{
      sst: sst,
      default_indicator: true,
      session: [{
        name: 'internet',
        type: 3,
        qos: {
          index: 9,
          arp: { priority_level: 8, pre_emption_capability: 1, pre_emption_vulnerability: 1 }
        },
        ambr: { downlink: { value: 1, unit: 3 }, uplink: { value: 1, unit: 3 } },
        pcc_rule: []
      }]
    }],
    __v: 0
  };
}

// ─── E1 + E3 — UE base srsRAN (4G y 5G) ───
db.subscribers.insertOne(make4GSubscriber('001011234567895', '4370816125816151'));

// ─── E4 — UE base UERANSIM gNB1 SST=1 ───
db.subscribers.insertOne(make5GSubscriber('001011234567896', '4370816125816152', 1));

// ─── E2 — UE invalido Ki incorrecto (credenciales correctas en DB, Ki malo en .conf) ───
db.subscribers.insertOne(make4GSubscriber('001011234567902', '4370816125816154'));

// ─── E4 — UE valido gNB2 SST=1 ───
db.subscribers.insertOne(make5GSubscriber('001011234567898', '4370816125816157', 1));

// ─── E4 — UE valido gNB2 SST=2 ───
db.subscribers.insertOne(make5GSubscriber('001011234567899', '4370816125816158', 2));

// ─── E4 — UE invalido K incorrecto (credenciales correctas en DB, K malo en .yaml) ───
db.subscribers.insertOne(make5GSubscriber('001011234567906', '4370816125816160', 1));

// ─── E4 — UE invalido DNN incorrecto (credenciales correctas en DB, DNN malo en .yaml) ───
db.subscribers.insertOne(make5GSubscriber('001011234567908', '4370816125816162', 1));

// ─── E4 — UE invalido SST=3 inexistente (credenciales correctas en DB, SST=3 en .yaml) ───
db.subscribers.insertOne(make5GSubscriber('001011234567909', '4370816125816165', 1));

// ─── NO insertar: 905 (bad_supi E4 — SUPI no registrado intencionalmente) ───

// ─── Verificar ───
print('Total subscribers:', db.subscribers.countDocuments());
db.subscribers.find({}, {imsi: 1, _id: 0}).sort({imsi: 1}).forEach(s => print(s.imsi));
"
