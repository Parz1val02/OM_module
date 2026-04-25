#!/usr/bin/bash
# Asegura que todos los ues necesarios para los escenarios están provisionados en mongo

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

// ─── Helper: suscriptor 5G con DNN 'internet' (SST=1 SD=000001) ───
function make5GSubscriber(imsi, imeisv, sst, sd) {
  var sliceEntry = {
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
  };
  if (sd !== undefined && sd !== null) {
    sliceEntry.sd = sd;
  }
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
    slice: [sliceEntry],
    __v: 0
  };
}

// ─── Helper: suscriptor 5G con DNN 'private' (SST=1 SD=000002) ───
function make5GPrivateSubscriber(imsi, imeisv) {
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
      sst: 1,
      sd: '000002',
      default_indicator: true,
      session: [{
        name: 'private',
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

// ─── E1 — UE base srsRAN 4G ───
db.subscribers.insertOne(make4GSubscriber('001011234567895', '4370816125816151'));

// ─── E2 — UEs multi-eNB 4G ───
db.subscribers.insertOne(make4GSubscriber('001011234567902', '4370816125816154'));

// ─── E2 — UE invalido APN incorrecto (credenciales correctas en DB, APN malo en .conf) ───
db.subscribers.insertOne(make4GSubscriber('001011234567903', '4370816125816155'));

// ─── E3 / E4 — UE base srsRAN 5G (srsgnb, SST=1 SD=000001, DNN=internet) ───
db.subscribers.insertOne(make5GSubscriber('001011234567895', '4370816125816151', 1, '000001'));

// ─── E3 / E4 — UE base UERANSIM nr_gnb SST=1 SD=000001 DNN=internet ───
db.subscribers.insertOne(make5GSubscriber('001011234567896', '4370816125816152', 1, '000001'));

// ─── E4 — UE valido nr_ue2 gNB2 SST=1 SD=000001 DNN=internet ───
db.subscribers.insertOne(make5GSubscriber('001011234567898', '4370816125816157', 1, '000001'));

// ─── E4 — UE valido nr_ue3 gNB2 SST=1 SD=000002 DNN=private (→ smf2/upf2) ───
db.subscribers.insertOne(make5GPrivateSubscriber('001011234567899', '4370816125816158'));

// ─── E4 — UE invalido bad_ki (Ki malo en .yaml, credenciales correctas en DB) ───
db.subscribers.insertOne(make5GSubscriber('001011234567906', '4370816125816160', 1, '000001'));

// ─── E4 — UE invalido bad_dnn (DNN malo en .yaml, credenciales correctas en DB) ───
db.subscribers.insertOne(make5GSubscriber('001011234567908', '4370816125816162', 1, '000001'));

// ─── E4 — UE invalido bad_sst (SD=000003 inexistente en .yaml, SD=000001 en DB) ───
db.subscribers.insertOne(make5GSubscriber('001011234567909', '4370816125816165', 1, '000001'));

// ─── NO insertar: 901 (bad_imsi E2 — IMSI no registrado intencionalmente) ───
// ─── NO insertar: 905 (bad_supi E4 — SUPI no registrado intencionalmente) ───

// ─── Verificar ───
print('Total subscribers:', db.subscribers.countDocuments());
db.subscribers.find({}, {imsi: 1, _id: 0}).sort({imsi: 1}).forEach(s => print(s.imsi));
"
