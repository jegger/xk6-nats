import { Nats } from 'k6/x/nats';
import { check, sleep } from 'k6';

const natsClient = new Nats({
  servers: 'nats://localhost:4222',
  credsfile: 'backend.creds'
});

export let options = {
    vus: 3,
    duration: '2s'
}

export default function () {
    const payload = {
        args:["62bd3e222522321933a078c4"], 
        kwargs:{timestamp:0}
    }
    ;

    const res = natsClient.request('piplanning.board.6.quartabiz.6e722482-5a95-47f3-94f4-f8168487c585.sticky.list', JSON.stringify(payload));
    check(res, {
        'payload pushed': (r) => r.data.length > 1,
    });
    sleep(1);
}

export function teardown() {
    natsClient.close();
}