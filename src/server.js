import sirv from 'sirv';
import express from 'express';
import compression from 'compression';
import * as sapper from '@sapper/server';
import { auth } from 'express-openid-connect';
import dotenv from 'dotenv';
import mongoose from 'mongoose';
import { json as jsonParser } from 'body-parser';

import { router as calender } from "./server/calender";

const APIS = [calender];

dotenv.config();
const { PORT, NODE_ENV, CLIENT_ID, CLIENT_SECRET, SECRET, BASE_URL, MONGO_DB_URL } = process.env;
const dev = NODE_ENV === 'development';

mongoose.connection.on('error', console.log);
mongoose.connect(MONGO_DB_URL, { keepAlive: 1, useCreateIndex: true, useNewUrlParser: true, useUnifiedTopology: true });

express()
	.disable('x-powered-by')
	.use(
		compression({ threshold: 0 }),
		sirv('static', { dev }),
		jsonParser(),
		auth({
			issuerBaseURL: 'https://auth.itemize.no',
			baseURL: BASE_URL || dev ? 'http://localhost:3000' : 'https://itemize.no',
			clientID: CLIENT_ID,
			clientSecret: CLIENT_SECRET,
			secret: SECRET,
			idpLogout: false,
			idTokenSigningAlg: 'HS256',
			authRequired: false,
		}),
		(req, res, next) => { // create req.user object
			const { name, roles, email, sub } = { roles: [], ...req.oidc?.user };
			req.user = { name, roles, email, id: sub };
			next();
		}
	)
	.get('/api/user', async (req, res) => {
		if (req.user?.name) {
			res.send({ "user": req.user });
		} else {
			res.send({});
		}
	})
	.use('/api', ...APIS)
	.use(sapper.middleware())
	.listen(PORT, err => {
		if (err) console.log('error', err);
	});
