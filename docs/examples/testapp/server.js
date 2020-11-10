'use strict';

const express = require('express');
// const promMid = require('express-prometheus-middleware');

// Constants
const PORT = 8080;
const HOST = '0.0.0.0';

// App
const app = express();
app.get('/', (req, res) => {
    res.send('Hello World');
});

// expose metrics:
// - https://www.npmjs.com/package/express-prometheus-middleware
// - https://medium.com/teamzerolabs/node-js-monitoring-with-prometheus-grafana-3056362ccb80
// app.use(promMid({
//     metricsPath: '/metrics',
//     collectDefaultMetrics: true,
//     requestDurationBuckets: [0.1, 0.5, 1, 1.5],
// }));

app.listen(PORT, HOST);
console.log(`Running on http://${HOST}:${PORT}`);

