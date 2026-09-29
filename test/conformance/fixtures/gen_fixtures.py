import json
import os
import struct
from pathlib import Path

out_dir = Path(r'D:\zyj_workspace\toy\xiaozhi-esp32-server-go\test\conformance\fixtures\protocol')
out_dir.mkdir(parents=True, exist_ok=True)

def make_fixture(name, msg_type, data_dict, expect_error=False):
    data_str = json.dumps(data_dict, separators=(',', ':'), ensure_ascii=False)
    hex_str = data_str.encode('utf-8').hex()
    fixture = {
        'name': name,
        'type': msg_type,
        'input_hex': hex_str,
        'expected': data_dict,
        'expect_error': expect_error
    }
    with open(out_dir / (name + '.json'), 'w', encoding='utf-8') as f:
        json.dump(fixture, f, separators=(',', ':'), ensure_ascii=False)

# Hello (5)
make_fixture('001_hello_basic_v1', 'hello', {'type':'hello','version':1,'transport':'websocket'})
make_fixture('002_hello_basic_v2', 'hello', {'type':'hello','version':2,'transport':'websocket'})
make_fixture('003_hello_with_features', 'hello', {'type':'hello','version':1,'features':{'mcp':True,'aec':True},'transport':'websocket'})
make_fixture('004_hello_with_all_features', 'hello', {'type':'hello','version':1,'features':{'mcp':False,'aec':True,'vad':True},'transport':'websocket'})
make_fixture('005_hello_with_audio_params', 'hello', {'type':'hello','version':1,'features':{},'transport':'websocket','audio_params':{'format':'opus','sample_rate':16000,'channels':1,'frame_duration':60}})

# Listen (10)
make_fixture('006_listen_start_auto', 'listen', {'type':'listen','session_id':'sess123','state':'start','mode':'auto'})
make_fixture('007_listen_start_manual', 'listen', {'type':'listen','session_id':'sess123','state':'start','mode':'manual'})
make_fixture('008_listen_start_realtime', 'listen', {'type':'listen','session_id':'sess123','state':'start','mode':'realtime'})
make_fixture('009_listen_stop_auto', 'listen', {'type':'listen','session_id':'sess123','state':'stop','mode':'auto'})
make_fixture('010_listen_stop_manual', 'listen', {'type':'listen','session_id':'sess123','state':'stop','mode':'manual'})
make_fixture('011_listen_detect', 'listen', {'type':'listen','session_id':'sess123','state':'detect','mode':'auto'})
make_fixture('012_listen_start_empty_text', 'listen', {'type':'listen','session_id':'sess123','state':'start','mode':'auto','text':''})
make_fixture('013_listen_start_manual_text', 'listen', {'type':'listen','session_id':'sess123','state':'start','mode':'manual','text':'wake word'})
make_fixture('014_listen_start_no_mode', 'listen', {'type':'listen','session_id':'sess123','state':'start'})
make_fixture('015_listen_stop_no_mode', 'listen', {'type':'listen','session_id':'sess123','state':'stop'})

# Abort (3)
make_fixture('016_abort_none', 'abort', {'type':'abort','session_id':'sess123','reason':'none'})
make_fixture('017_abort_wake_word', 'abort', {'type':'abort','session_id':'sess123','reason':'wake_word_detected'})
make_fixture('018_abort_no_session', 'abort', {'type':'abort','reason':'none'})

# ACK (5)
make_fixture('019_ack_basic', 'ack', {'type':'ack','sessionId':'sess123','msgType':'binary','msgId':'frame-1','status':'ok'})
make_fixture('020_ack_json_msg', 'ack', {'type':'ack','sessionId':'sess123','msgType':'json','msgId':'msg-2','status':'ok'})
make_fixture('021_ack_with_error', 'ack', {'type':'ack','sessionId':'sess123','msgType':'binary','msgId':'frame-3','status':'error','error':'decode failed'})
make_fixture('022_ack_batch', 'ack', {'type':'ack','sessionId':'sess123','msgType':'binary','msgId':'batch-1','status':'ok'})
make_fixture('023_ack_no_session', 'ack', {'type':'ack','msgType':'binary','msgId':'frame-1','status':'ok'})

# TTS (8)
make_fixture('024_tts_start', 'tts', {'type':'tts','state':'start'})
make_fixture('025_tts_sentence_start', 'tts', {'type':'tts','state':'sentence_start','text':'Hello world.'})
make_fixture('026_tts_sentence_end', 'tts', {'type':'tts','state':'sentence_end','text':'Hello world.'})
make_fixture('027_tts_stop', 'tts', {'type':'tts','state':'stop'})
make_fixture('028_tts_stop_reason', 'tts', {'type':'tts','state':'stop','reason':'wake_word_detected'})
make_fixture('029_tts_start_empty', 'tts', {'type':'tts','state':'start','text':''})
make_fixture('030_tts_sentence_cjk', 'tts', {'type':'tts','state':'sentence_start','text':'this is cjk'})
make_fixture('031_tts_start_unicode', 'tts', {'type':'tts','state':'start','text':'Testing unicode'})

# STT (5)
make_fixture('032_stt_basic', 'stt', {'type':'stt','text':'hello server'})
make_fixture('033_stt_empty_text', 'stt', {'type':'stt','text':'','emotion':'neutral'})
make_fixture('034_stt_happy', 'stt', {'type':'stt','text':'I am happy','emotion':'happy'})
make_fixture('035_stt_cjk', 'stt', {'type':'stt','text':'test chinese'})
make_fixture('036_stt_no_text', 'stt', {'type':'stt','emotion':'neutral'})

# LLM (5)
make_fixture('037_llm_neutral', 'llm', {'type':'llm','emotion':'neutral','action':'speak','text':'hello'})
make_fixture('038_llm_happy', 'llm', {'type':'llm','emotion':'happy','action':'speak','text':'great'})
make_fixture('039_llm_sad', 'llm', {'type':'llm','emotion':'sad','action':'speak','text':'sorry'})
make_fixture('040_llm_excited', 'llm', {'type':'llm','emotion':'excited','action':'speak','text':'wow'})
make_fixture('041_llm_calm', 'llm', {'type':'llm','emotion':'calm','action':'speak','text':'slow'})

# MCP (10)
make_fixture('042_mcp_status', 'mcp', {'type':'mcp','session_id':'sess123','payload':{'jsonrpc':'2.0','method':'status','id':1}})
make_fixture('043_mcp_tools_list', 'mcp', {'type':'mcp','session_id':'sess123','payload':{'jsonrpc':'2.0','method':'tools/list','id':2,'params':{}}})
make_fixture('044_mcp_tools_call', 'mcp', {'type':'mcp','session_id':'sess123','payload':{'jsonrpc':'2.0','method':'tools/call','id':3,'params':{'name':'test','arguments':{}}}})
make_fixture('045_mcp_result', 'mcp', {'type':'mcp','session_id':'sess123','payload':{'jsonrpc':'2.0','result':{'tools':[]},'id':4}})
make_fixture('046_mcp_error', 'mcp', {'type':'mcp','session_id':'sess123','payload':{'jsonrpc':'2.0','error':{'code':-32600,'message':'Invalid Request'},'id':5}})
make_fixture('047_mcp_initialize', 'mcp', {'type':'mcp','session_id':'sess123','payload':{'jsonrpc':'2.0','method':'initialize','id':6,'params':{'protocolVersion':'1.0','capabilities':{},'clientInfo':{'name':'test','version':'1.0'}}}})
make_fixture('048_mcp_ping', 'mcp', {'type':'mcp','session_id':'sess123','payload':{'jsonrpc':'2.0','method':'ping','id':7}})
make_fixture('049_mcp_notification', 'mcp', {'type':'mcp','payload':{'jsonrpc':'2.0','method':'notifications/initialized','id':None}})
make_fixture('050_mcp_resources_list', 'mcp', {'type':'mcp','session_id':'sess123','payload':{'jsonrpc':'2.0','method':'resources/list','id':8,'params':{'cursor':None}}})
make_fixture('051_mcp_prompts_list', 'mcp', {'type':'mcp','session_id':'sess123','payload':{'jsonrpc':'2.0','method':'prompts/list','id':9,'params':{}}})

# IoT (5)
make_fixture('052_iot_power_on', 'iot', {'type':'iot','session_id':'sess123','msgId':'iot-1','state':{'power':'on'}})
make_fixture('053_iot_power_off', 'iot', {'type':'iot','session_id':'sess123','msgId':'iot-2','state':{'power':'off','brightness':0}})
make_fixture('054_iot_color', 'iot', {'type':'iot','session_id':'sess123','msgId':'iot-3','state':{'color':'#FF0000','brightness':100}})
make_fixture('055_iot_no_session', 'iot', {'type':'iot','state':{'power':'on'}})
make_fixture('056_iot_thermostat', 'iot', {'type':'iot','session_id':'sess123','state':{'mode':'auto','temperature':22}})

# Alert (3)
make_fixture('057_alert_info', 'alert', {'type':'alert','status':'info','message':'Device online'})
make_fixture('058_alert_warning', 'alert', {'type':'alert','status':'warning','message':'Low battery'})
make_fixture('059_alert_error', 'alert', {'type':'alert','status':'error','message':'Connection lost'})

# System (3)
make_fixture('060_system_session_start', 'system', {'type':'system','event':'session_start','session_id':'sess123'})
make_fixture('061_system_session_end', 'system', {'type':'system','event':'session_end','session_id':'sess123'})
make_fixture('062_system_heartbeat', 'system', {'type':'system','event':'heartbeat','session_id':'sess123'})

# Binary v2 (15)
def make_binary_v2(name, frame_type, timestamp, payload_bytes):
    header = struct.pack('<HHIII', 2, frame_type, 0, timestamp, len(payload_bytes))
    data = header + payload_bytes
    hex_str = data.hex()
    fixture = {
        'name': name,
        'type': 'binary_v2',
        'input_hex': hex_str,
        'expected': {'version': 2, 'type': frame_type, 'timestamp': timestamp, 'payload_size': len(payload_bytes)},
        'expect_error': False
    }
    with open(out_dir / (name + '.json'), 'w', encoding='utf-8') as f:
        json.dump(fixture, f, separators=(',', ':'))

make_binary_v2('063_binary_v2_opus', 0, 0, b'opus-data')
make_binary_v2('064_binary_v2_json', 1, 0, b'{"type":"mcp"}')
make_binary_v2('065_binary_v2_ts12345', 0, 12345, b'test')
make_binary_v2('066_binary_v2_type2', 2, 0, b'data')
make_binary_v2('067_binary_v2_type3', 3, 0, b'data')
make_binary_v2('068_binary_v2_large', 0, 0, b'x' * 256)
make_binary_v2('069_binary_v2_empty', 0, 0, b'')
make_binary_v2('070_binary_v2_all_types_0', 0, 100, b't0')
make_binary_v2('071_binary_v2_all_types_1', 1, 101, b't1')
make_binary_v2('072_binary_v2_all_types_2', 2, 102, b't2')
make_binary_v2('073_binary_v2_all_types_3', 3, 103, b't3')
make_binary_v2('074_binary_v2_all_types_4', 4, 104, b't4')
make_binary_v2('075_binary_v2_ts_max', 0, 0xFFFFFFFF, b'data')
make_binary_v2('076_binary_v2_type5', 5, 0, b't5')
make_binary_v2('077_binary_v2_type6', 6, 0, b't6')
make_binary_v2('078_binary_v2_type7', 7, 0, b't7')
make_binary_v2('079_binary_v2_type8', 8, 0, b't8')

def make_binary_v2_err(name, frame_type, timestamp, declared_size, actual_payload):
    header = struct.pack('<HHIII', 2, frame_type, 0, timestamp, declared_size)
    data = header + actual_payload
    hex_str = data.hex()
    fixture = {
        'name': name,
        'type': 'binary_v2',
        'input_hex': hex_str,
        'expected': {'error': 'payload_size_mismatch'},
        'expect_error': True
    }
    with open(out_dir / (name + '.json'), 'w', encoding='utf-8') as f:
        json.dump(fixture, f, separators=(',', ':'))

make_binary_v2_err('080_binary_v2_payload_short', 0, 0, 100, b'short')

short_header = struct.pack('<HHIII', 2, 0, 0, 0, 0)[:10]
fixture = {'name':'081_binary_v2_short_frame','type':'binary_v2','input_hex':short_header.hex(),'expected':{'error':'short_frame'},'expect_error':True}
with open(out_dir / '081_binary_v2_short_frame.json', 'w', encoding='utf-8') as f:
    json.dump(fixture, f, separators=(',', ':'))

# Binary v3 (15)
def make_binary_v3(name, frame_type, payload_bytes):
    header = struct.pack('<BBH', frame_type, 0, len(payload_bytes))
    data = header + payload_bytes
    hex_str = data.hex()
    fixture = {
        'name': name,
        'type': 'binary_v3',
        'input_hex': hex_str,
        'expected': {'type': frame_type, 'payload_size': len(payload_bytes)},
        'expect_error': False
    }
    with open(out_dir / (name + '.json'), 'w', encoding='utf-8') as f:
        json.dump(fixture, f, separators=(',', ':'))

make_binary_v3('077_binary_v3_opus', 0, b'opus-payload')
make_binary_v3('078_binary_v3_json', 1, b'{"a":1}')
make_binary_v3('079_binary_v3_type2', 2, b'data')
make_binary_v3('080_binary_v3_type3', 3, b'data')
make_binary_v3('081_binary_v3_type4', 4, b'data')
make_binary_v3('082_binary_v3_empty', 0, b'')
make_binary_v3('083_binary_v3_large', 0, b'x' * 512)
make_binary_v3('084_binary_v3_1b', 0, b'x')
make_binary_v3('085_binary_v3_ts0', 1, b'test')
make_binary_v3('086_binary_v3_ts1', 1, b'{"json":"data"}')
make_binary_v3('087_binary_v3_all_types_0', 0, b'0')
make_binary_v3('088_binary_v3_all_types_1', 1, b'1')
make_binary_v3('089_binary_v3_all_types_2', 2, b'2')
make_binary_v3('090_binary_v3_all_types_3', 3, b'3')
make_binary_v3('091_binary_v3_all_types_4', 4, b'4')

def make_binary_v3_err(name, frame_type, declared_size, actual_payload):
    header = struct.pack('<BBH', frame_type, 0, declared_size)
    data = header + actual_payload
    hex_str = data.hex()
    fixture = {
        'name': name,
        'type': 'binary_v3',
        'input_hex': hex_str,
        'expected': {'error': 'payload_size_mismatch'},
        'expect_error': True
    }
    with open(out_dir / (name + '.json'), 'w', encoding='utf-8') as f:
        json.dump(fixture, f, separators=(',', ':'))

make_binary_v3_err('094_binary_v3_payload_short', 0, 100, b'short')

short_header3 = struct.pack('<BBH', 0, 0, 0)[:3]
fixture = {'name':'095_binary_v3_short_frame','type':'binary_v3','input_hex':short_header3.hex(),'expected':{'error':'short_frame'},'expect_error':True}
with open(out_dir / '095_binary_v3_short_frame.json', 'w', encoding='utf-8') as f:
    json.dump(fixture, f, separators=(',', ':'))

# Malformed/error cases (8) - protocol-level validation errors
make_fixture('096_malformed_version', 'error', {'type':'hello','version':99}, True)
make_fixture('097_malformed_unknown_type', 'error', {'type':'unknown_type','version':1}, True)
make_fixture('098_malformed_invalid_listen_state', 'error', {'type':'listen','session_id':'sess123','state':'invalid_state'}, True)
make_fixture('099_malformed_invalid_listen_mode', 'error', {'type':'listen','session_id':'sess123','mode':'invalid_mode'}, True)
make_fixture('100_malformed_invalid_tts_state', 'error', {'type':'tts','state':'invalid_state'}, True)
make_fixture('101_malformed_features_not_object', 'error', {'type':'hello','version':1,'features':'not-object'}, True)
make_fixture('102_malformed_audio_params_not_object', 'error', {'type':'hello','audio_params':'not-object'}, True)
make_fixture('103_malformed_mcp_payload_not_object', 'error', {'type':'mcp','session_id':'sess123','payload':'not-object'}, True)

# Binary v1 (5)
def make_binary_v1(name, payload_bytes):
    hex_str = payload_bytes.hex()
    fixture = {
        'name': name,
        'type': 'binary_v1',
        'input_hex': hex_str,
        'expected': {'payload_size': len(payload_bytes)},
        'expect_error': False
    }
    with open(out_dir / (name + '.json'), 'w', encoding='utf-8') as f:
        json.dump(fixture, f, separators=(',', ':'))

make_binary_v1('104_binary_v1_type0', bytes([0]))
make_binary_v1('105_binary_v1_type1', bytes([1]))
make_binary_v1('106_binary_v1_multi', bytes([2, 3, 4, 5]))
make_binary_v1('107_binary_v1_type255', bytes([255]))
make_binary_v1('108_binary_v1_100bytes', b'x' * 100)

files = sorted(out_dir.glob('*.json'))
print('Total fixtures:', len(files))
for f in files:
    print(' ', f.name)
