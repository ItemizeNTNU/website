import mongoose from 'mongoose';
import express from 'express';
import { permission } from './utils';
import { deleteDiscordEvent, upsertDiscordEvent } from './discord';
import ical from 'ical-generator';
import joi from 'joi';
import { DateTime } from 'luxon';
import { v4 as uuidv4 } from 'uuid';

export const router = express.Router();

const validURL = (url) => {
	if (url !== '') {
		if (!String(url).startsWith('https://')) {
			throw Error('the URL needs to start with https://');
		}
		if (String(url).split(/\s/).length > 1) {
			throw Error('the URL can not include whitespace');
		}
	}
	return url;
};

const eventValidationSchema = joi.object({
	_id: joi.string().regex(/[0-9a-f]{24}/),
	name: joi.string().trim().min(3).max(50).required(),
	location: {
		name: joi.string().trim().min(3).max(200).required(),
		url: joi.string().trim().allow('').custom(validURL).required()
	},
	register_url: joi.string().allow('').trim().custom(validURL).required(),
	date: joi.date().required(),
	duration: joi.number().min(0).required(),
	ctf: {
		name: joi.string().trim().allow('').max(200).required(),
		url: joi.string().trim().allow('').custom(validURL).required()
	},
	info: joi.string().trim().min(3).max(2000).required(),
	hidden: joi.boolean().required(),
	discord: joi.boolean().required(),
	discordEventId: joi
		.string()
		.allow('')
		.trim()
		.regex(/[0-9]*/),
	check_in: {
		code: joi.string().trim(),
		attendances: joi.array().items(joi.any()).allow(null)
	}
});

const eventSchema = mongoose.Schema({
	name: String,
	location: {
		name: String,
		url: String
	},
	register_url: String,
	date: Date,
	duration: Number,
	end: Date,
	ctf: {
		name: String,
		url: String
	},
	info: String,
	hidden: Boolean,
	discord: Boolean,
	discordEventId: String,
	created: { type: Date, default: Date.now },
	edited: Date,
	check_in: {
		code: String,
		attendances: [
			{
				name: String,
				user_id: String,
				registered: { type: Date, default: Date.now }
			}
		]
	}
});
eventSchema.index({ date: 1 });
const Event = mongoose.model('events', eventSchema);

router.get('/events', async (req, res) => {
	const page = Math.max((req.query.page || 1) - 1, 0);
	const includeOld = ['true', '1', ''].includes(req.query.old);
	const count = 100;
	const search = {};
	if (!includeOld) {
		search.end = { $gt: Date.now() - 1000 * 60 * 60 * 6 };
	}
	if (!req.user?.roles?.includes('Styret')) {
		search.hidden = 0;
	}
	res.send(
		await Event.find(search)
			.sort({ date: 1 })
			.skip(page * count)
			.limit(count)
			.lean()
	);
});
router.post('/events', permission('Styret'), async (req, res) => {
	let event = req.body;
	delete event.created;
	delete event.edited;
	delete event.end;
	event.discordEventId = '';
	event.check_in = { code: 'null' };
	if (event._id) {
		const oldExisting = await Event.findById({ _id: event._id });
		event.check_in = oldExisting.check_in || { code: 'null' };
		event.check_in = { ...event.check_in, attendances: event.check_in.attendances ?? undefined };
		event.discordEventId = oldExisting.discordEventId || '';
	}
	event = eventValidationSchema.validate(event, { abortEarly: true, convert: true, stripUnknown: true });
	if (event.error) {
		return res.status(400).send({ message: event.error.details[0].message });
	}
	event = event.value;
	event.end = DateTime.fromJSDate(event.date).plus({ hours: event.duration }).toJSDate();
	const resp = { message: 'Success' };
	if (!event._id) {
		event.edited = Date.now();
	} else {
		event.created = Date.now();
	}
	if (event.check_in?.code == 'null') event.check_in = { code: uuidv4() + '', attendances: undefined };
	if ((event.hidden || !event.discord) && event.discordEventId) {
		console.log('deleting discord event...');
		await deleteDiscordEvent(event.discordEventId);
		event.discordEventId = '';
	} else if (!event.hidden && event.discord) {
		const discordEvent = await upsertDiscordEvent(event);
		if (discordEvent.error) {
			console.error('Error upserting discord event:', discordEvent.error);
			resp.message = `Error upserting discord event: ${discordEvent.error}`;
			resp.error = true;
		} else {
			event.discordEventId = discordEvent.json.id;
		}
	}
	let _id = event._id || mongoose.Types.ObjectId();
	delete event._id;
	await Event.updateOne({ _id }, event, { upsert: true });
	res.status(resp.error ? 500 : 200).send(resp);
});

router.delete('/events/:id', permission('Styret'), async (req, res) => {
	const _id = req.params.id;
	const event = await Event.findById(_id);
	if (event.discord && !event.hidden && event.discordEventId) {
		const discordDeletion = await deleteDiscordEvent(event.discordEventId);
		if (discordDeletion.exception) {
			return res.status(500).send({ message: `Error trying to delete discord event: ${discordDeletion.error}` });
		}
	}
	const eventDeletion = await Event.deleteOne({ _id });
	if (!eventDeletion.deletedCount) {
		res.send({ message: 'Success' });
	} else {
		res.status(404).send({ message: `Event not found with id "${_id}"` });
	}
});

router.get('/events/ical', async (req, res) => {
	const cal = ical({ name: 'Itemize Arrangementer' });
	const events = await Event.find({ hidden: 0 }).lean();
	events.forEach((e) => {
		cal.createEvent({
			id: e._id,
			start: e.date,
			end: e.end,
			summary: e.name,
			description: e.info,
			location: e.location.name,
			url: 'https://itemize.no/arrangementer' // TODO: Add direct link to event
		});
	});
	cal.serve(res);
});

router.get('/checkin/:code', async (req, res) => {
	const code = req.params.code;
	res.send(await Event.findOne({ 'check_in.code': code }).lean());
});

router.post('/checkin/:code', async (req, res) => {
	const code = req.params.code;
	const event = await Event.findOne({ 'check_in.code': code }).lean();
	if (!event) return res.status(404).send({ message: `Event not found with check_in code "${code}"` });

	const _id = event._id || mongoose.Types.ObjectId();
	delete event._id;

	const resp = { message: 'Success' };
	const user_id = req.user?.id;
	if (user_id) {
		const nAttendance = { name: req.user.fullName, user_id: user_id, registered: Date.now() };
		const attendances = event.check_in.attendances ? [...event.check_in.attendances, nAttendance] : [nAttendance];
		if (attendances.filter((na) => na?.user_id == user_id).length > 1)
			return res.status(500).send({ message: `You have already registered your attendance for event "${event.name}"` });
		event.check_in = { ...event.check_in, attendances };
		await Event.updateOne({ _id }, event, { upsert: true });
	} else {
		resp.message = `User id not found`;
		resp.error = true;
	}
	res.status(resp.error ? 500 : 200).send(resp);
});
