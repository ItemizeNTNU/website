import mongoose from 'mongoose';
import express from 'express';
import { permission } from './utils';
import ical from 'ical-generator';
import joi from 'joi';
import { DateTime } from 'luxon';

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
	hidden: joi.boolean().required()
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
	created: { type: Date, default: Date.now },
	edited: Date
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
	event = eventValidationSchema.validate(event, { abortEarly: true, convert: true, stripUnknown: true });
	if (event.error) {
		return res.status(400).send({ message: event.error.details[0].message });
	}
	event = event.value;
	event.end = DateTime.fromJSDate(event.date).plus({ hours: event.duration }).toJSDate();
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

router.delete('/events/:id', permission('Styret'), async (req, res) => {
	const _id = req.params.id;
	const del = await Event.deleteOne({ _id });
	if (del.deletedCount) {
		res.send({ message: 'Success' });
	} else {
		res.status(404).status({ message: `Event not found with id "${_id}"` });
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
