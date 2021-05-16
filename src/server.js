import sirv from 'sirv';
import express from 'express';
import compression from 'compression';
import * as sapper from '@sapper/server';
import { auth } from 'express-openid-connect';
import dotenv from 'dotenv';

dotenv.config();
const { PORT, NODE_ENV, CLIENT_ID, CLIENT_SECRET } = process.env;
const dev = NODE_ENV === 'development';

express()
	.disable('x-powered-by')
	.use(
		compression({ threshold: 0 }),
		sirv('static', { dev }),
		auth({
			issuerBaseURL: 'https://auth.itemize.no',
			baseURL: dev ? 'http://localhost:3000' : 'https://itemize.no',
			clientID: CLIENT_ID,
			secret: CLIENT_SECRET,
			idpLogout: false,
			idTokenSigningAlg: 'HS256',
			authRequired: false,
		}),
		(req, res, next) => { // create req.user object
			const user = req.oidc?.user;
			if (user) {
				const { name, roles, email } = req.oidc?.user;
				req.user = { name, roles, email };
			}
			next();
		}
	)
	.get('/api/user', async (req, res) => {
		res.send({ "user": req.user });
	})
	.use(sapper.middleware())
	.listen(PORT, err => {
		if (err) console.log('error', err);
	});
