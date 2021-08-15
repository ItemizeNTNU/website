import express from "express";
import joi from 'joi';
import { permission, removeEmptyObjects } from "./utils";
import fusion from '../server/fusion'

export const router = express.Router();

const validateNTNUEmail = e => {
	if (String(e).toLowerCase().endsWith('@stud.ntnu.no')) {
		throw Error('Vennligst ikke bruk din stud e-post adresse, da du mister tilgang til denne etter fullført utdannelse.');
	}
	return e;
}

const UserSchema = joi.object({
	fullName: joi.string().min(3).max(64).required(),
	email: joi.string().email().custom(validateNTNUEmail).required(),
	data: {
		displayName: joi.string().min(3).max(32).required(),
		type: joi.string().allow('student', 'alumni', 'employee').required(),
		study: {
			program: joi.when('$type', { is: ['alumni', 'student'], then: joi.string().min(3).max(64).required(), otherwise: joi.strip() }),
			year: joi.when('$type', { is: 'student', then: joi.number().integer().min(1).max(100).required(), otherwise: joi.strip() }),
		},
		alumni: {
			joinYear: joi.when('$type', { is: 'alumni', then: joi.number().integer().min(2014).max(new Date().getFullYear()).required(), otherwise: joi.strip() }),
		},
		employee: {
			title: joi.when('$type', { is: 'employee', then: joi.string().min(3).max(64).required(), otherwise: joi.strip() })
		},
	}
});

const validate_user = data => {
	const user = UserSchema.validate(data, { abortEarly: true, convert: true, stripUnknown: true, context: { type: data.data?.type } });
	if (!user.error) {
		removeEmptyObjects(user.value);
	}
	return user;
};

router.put('/user', async (req, res) => {
	if (req.user) {
		return res.status(400).send({ message: 'You are already registered' });
	}
	const user = validate_user(req.body);
	if (user.error) {
		return res.status(400).send({ message: user.error.details[0].message });
	}

	const resp = await fusion.createUser({
		sendSetPasswordEmail: true,
		user: user.value,
	});
	return res.status(resp.status).send(resp.client);
});

router.post('/events', permission('Styret'), async (req, res) => {
	const event = req.body;
	delete event.created;
	delete event.edited;
	if (!event._id) {
		event.edited = Date.now();
	} else {
		event.created = Date.now();
	}
	let _id = event._id || mongoose.Types.ObjectId();
	delete event._id;
	await Event.updateOne({ _id }, event, { upsert: true });
	res.send();
});