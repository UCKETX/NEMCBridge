const $=id=>document.getElementById(id);
let state=null,loaded=false,lastEvents='',message='';
const phases={stopped:'已停止',checking_upstream:'检查本地服务器',authenticating:'正在鉴权',creating:'正在创建房间',online:'房间在线',retrying:'等待自动重连',upstream_offline:'本地服务器离线'};
function notice(text){$('notice').textContent=text;$('notice').style.display=text?'block':'none'}
async function api(path,data){const r=await fetch('/api/'+path,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(data??{})});if(r.status===401){window.location.replace('/login');throw Error('登录已过期，请重新登录')}if(!r.ok)throw Error(await r.text());return r.json()}
function loadForm(c){$('auth-server').value=c.auth_server;$('server').value=c.upstream;$('room-name').value=c.room_name;$('room-game-version').value=c.room_game_version;$('password').value=c.room_password;$('max-players').value=c.max_players;$('friends').checked=c.friends_only;$('pvp').checked=c.allow_pvp;$('auto').checked=c.auto_start}
async function refresh(){try{
const r=await fetch('/api/status');if(r.status===401){window.location.replace('/login');return}if(!r.ok)throw Error('控制台暂不可用');state=await r.json();if(!loaded){loadForm(state.config);loaded=true}
$('phase').textContent=phases[state.phase]??state.phase;$('room-id').textContent=state.room_id||'—';$('room-note').textContent=state.room_id?'在网易我的世界“本地联机”中搜索此房间号。':'启动房主机器人后，房间号会显示在这里。';$('upstream').textContent=state.upstream_online?'在线':'未连接';$('version').textContent=state.config.upstream+(state.upstream_version?' · '+state.upstream_version:'');$('account').textContent=state.account||'—';$('players').textContent=state.players;$('protocol').textContent=state.protocol_id?'房间协议 '+state.protocol_id:'等待鉴权';$('token-note').textContent=state.token_set?'已保存':'尚未设置';
const seconds=state.room_id&&state.connected_at?Math.max(0,Math.floor((Date.now()-Date.parse(state.connected_at))/1000)):0;$('uptime').textContent=state.room_id?Math.floor(seconds/3600)+'时 '+Math.floor(seconds%3600/60)+'分':'—';$('retry').textContent=state.retry_at?'将在 '+new Date(state.retry_at).toLocaleTimeString()+' 重试':'自动断线重连已启用';
$('start').disabled=state.running||!state.token_set;$('stop').disabled=!state.running;$('copy').disabled=!state.room_id;document.querySelectorAll('#config input,#save').forEach(x=>x.disabled=state.running);
notice(message||state.last_error);
const key=JSON.stringify(state.events);if(key!==lastEvents){lastEvents=key;const box=$('events');const atBottom=box.scrollHeight-box.scrollTop-box.clientHeight<40;box.replaceChildren();for(const e of state.events){const row=document.createElement('div');row.className='event';const time=document.createElement('time');time.textContent=e.time;row.append(time,document.createTextNode(e.message));box.append(row)}if(atBottom)box.scrollTop=box.scrollHeight}
}catch(e){notice(e.message)}}
$('start').onclick=async()=>{try{message='';await api('start');await refresh()}catch(e){notice(e.message)}};
$('stop').onclick=async()=>{try{message='';await api('stop');await refresh()}catch(e){notice(e.message)}};
$('copy').onclick=async()=>{try{await navigator.clipboard.writeText(String(state.room_id));$('copy').textContent='已复制';setTimeout(()=>$('copy').textContent='复制房间号',1500)}catch{notice('请手动复制上方房间号')}};
$('config').onsubmit=async e=>{e.preventDefault();try{const config={...state.config,auth_server:$('auth-server').value.trim(),upstream:$('server').value.trim(),room_name:$('room-name').value.trim(),room_game_version:$('room-game-version').value.trim(),room_password:$('password').value,max_players:Number($('max-players').value),friends_only:$('friends').checked,allow_pvp:$('pvp').checked,auto_start:$('auto').checked};const body={config};if($('token').value.trim())body.token=$('token').value.trim();await api('config',body);$('token').value='';message='设置已保存';setTimeout(()=>{message=''},3500);await refresh()}catch(e){notice(e.message)}};
refresh();setInterval(refresh,2000);

$('logout').onclick=async()=>{try{await api('logout');window.location.replace('/login')}catch(e){notice(e.message)}};
