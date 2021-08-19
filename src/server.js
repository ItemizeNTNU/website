import sirv from 'sirv';
import express from 'express';
import compression from 'compression';
import * as sapper from '@sapper/server';
import { auth } from 'express-openid-connect';
import mongoose from 'mongoose';
import { json as jsonParser } from 'body-parser';

import './server/load_envs';
import { router as calender } from './server/calender';
import { router as user } from './server/user';

const APIS = [calender, user];

const { PORT, NODE_ENV, FUSION_AUTH_CLIENT_ID, FUSION_AUTH_CLIENT_SECRET, FUSION_AUTH_SECRET, BASE_URL, MONGO_DB_URL } = process.env;
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
			clientID: FUSION_AUTH_CLIENT_ID,
			clientSecret: FUSION_AUTH_CLIENT_SECRET,
			secret: FUSION_AUTH_SECRET,
			idpLogout: false,
			idTokenSigningAlg: 'HS256',
			authRequired: false
		}),
		(req, _, next) => {
			// create req.user object
			const { name, roles, email, sub, imageUrl, fullName } = { roles: [], ...req.oidc?.user };
			if (name) req.user = { name, roles, email, id: sub, imageUrl, fullName };
			next();
		}
	)
	.use('/api', ...APIS)
	.use('/api*', (req, res) => {
		res.status(404).send({ message: 'API endpoint not found' });
	})
	.use(
		sapper.middleware({
			session: (req) => ({
				user: req.user?.name ? req.user : undefined
			})
		})
	)
	.use((err, req, res) => {
		console.error('Error:', err.stack);
		try {
			if (!res.headersSent) res.status(500);
			res.send({ message: 'Something broke :/' });
		} catch {
			if (!res.writableEnded) res.end();
		}
	})
	.listen(PORT, (err) => {
		if (err) console.error('error', err);
	});
