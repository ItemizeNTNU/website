import mongoose from 'mongoose';
import express from "express";
import { permission } from "./utils";

export const router = express.Router();

const eventSchema = mongoose.Schema({
	name: String,
	location: {
		name: String,
		url: String,
	},
	register_url: String,
	date: Date,
	ctf: {
		name: String,
		url: String,
	},
	info: String,
	hidden: Boolean,
	created: { type: Date, default: Date.now },
	edited: Date,
});
eventSchema.index({ date: 1 });
const Event = mongoose.model('events', eventSchema);

router.get('/events', async (req, res) => {
	const page = Math.max((req.query.page || 1) - 1, 0);
	const count = 100;
	const search = { date: { $gt: Date.now() - 1000 * 60 * 60 * 6 } };
	if (!req.user.roles.includes("Styret")) {
		search.hidden = 0;
	}
	res.send(await Event.find(search).sort({ date: 1 }).skip(page * count).limit(count).lean());
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