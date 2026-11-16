package notify

import "html/template"

// Email-safe HTML: tables + inline styles, matching the site's calm palette.
const layout = `{{define "layout"}}<!doctype html>
<html><body style="margin:0;background:#f7f6f2;font-family:-apple-system,Segoe UI,Inter,Arial,sans-serif;color:#1f2421">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#f7f6f2;padding:32px 12px"><tr><td align="center">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:520px;background:#ffffff;border:1px solid #e4e1d9;border-radius:16px;overflow:hidden">
<tr><td style="background:#1c4236;padding:22px 28px;color:#ffffff;font-family:Georgia,serif;font-size:20px">EventHub</td></tr>
<tr><td style="padding:28px">{{template "body" .}}</td></tr>
</table>
<p style="color:#8b918c;font-size:12px;margin:18px 0 0">Fair, first-come seating. Every seat is sold exactly once.</p>
</td></tr></table></body></html>{{end}}`

const detailRows = `{{define "details"}}
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="font-size:14px;line-height:1.5;margin:18px 0">
<tr><td style="color:#8b918c;padding:6px 0;width:90px">When</td><td style="padding:6px 0">{{.When}}</td></tr>
<tr><td style="color:#8b918c;padding:6px 0">Where</td><td style="padding:6px 0">{{.Venue}}<br><a href="{{.DirectionsURL}}" style="color:#245544">Get directions</a></td></tr>
<tr><td style="color:#8b918c;padding:6px 0">Tickets</td><td style="padding:6px 0">{{.Items}}</td></tr>
<tr><td style="color:#8b918c;padding:6px 0">Paid</td><td style="padding:6px 0">{{.Total}}</td></tr>
</table>{{end}}`

var ticketTmpl = template.Must(template.New("ticket").Parse(layout + detailRows + `{{define "body"}}
<p style="margin:0 0 6px;color:#5c635e">Hi {{.Name}}, you're going!</p>
<h1 style="margin:0;font-family:Georgia,serif;font-weight:500;font-size:24px">{{.EventTitle}}</h1>
{{template "details" .}}
<div style="text-align:center;border-top:2px dashed #e4e1d9;padding-top:22px">
  <img src="{{.QRURL}}" width="200" height="200" alt="Ticket QR code" style="display:block;margin:0 auto">
  <p style="margin:10px 0 0;font-family:Menlo,monospace;font-size:20px;letter-spacing:2px">{{.Code}}</p>
  <p style="margin:6px 0 0;color:#8b918c;font-size:13px">Show this QR code at the entrance.</p>
</div>
<p style="text-align:center;margin:24px 0 0"><a href="{{.TicketURL}}" style="background:#245544;color:#fff;text-decoration:none;padding:12px 22px;border-radius:8px;font-size:14px">View ticket</a></p>
{{end}}{{template "layout" .}}`))

var hostTmpl = template.Must(template.New("host").Parse(layout + detailRows + `{{define "body"}}
<p style="margin:0 0 6px;color:#5c635e">New booking for</p>
<h1 style="margin:0;font-family:Georgia,serif;font-weight:500;font-size:24px">{{.EventTitle}}</h1>
<p style="margin:14px 0 0;font-size:15px"><strong>{{.Name}}</strong> booked {{.Count}} ticket{{if ne .Count 1}}s{{end}}.</p>
{{template "details" .}}
<p style="margin:0;padding:14px 16px;background:#eef5f1;border-radius:10px;font-size:14px;color:#1c4236">So far: <strong>{{.TotalBookings}}</strong> bookings · <strong>{{.TotalRevenue}}</strong></p>
<p style="text-align:center;margin:24px 0 0"><a href="{{.HostURL}}" style="background:#245544;color:#fff;text-decoration:none;padding:12px 22px;border-radius:8px;font-size:14px">Open dashboard</a></p>
{{end}}{{template "layout" .}}`))

var cancelTmpl = template.Must(template.New("cancel").Parse(layout + detailRows + `{{define "body"}}
<p style="margin:0 0 6px;color:#5c635e">Hi {{.Name}}, we're sorry.</p>
<h1 style="margin:0;font-family:Georgia,serif;font-weight:500;font-size:24px">{{.EventTitle}} has been cancelled</h1>
<p style="margin:14px 0 0;font-size:15px">The organiser cancelled this event. Your payment of <strong>{{.Total}}</strong> has been refunded in full and your ticket <span style="font-family:Menlo,monospace">{{.Code}}</span> is no longer valid.</p>
<p style="text-align:center;margin:24px 0 0"><a href="{{.SiteURL}}" style="background:#245544;color:#fff;text-decoration:none;padding:12px 22px;border-radius:8px;font-size:14px">Find another event</a></p>
{{end}}{{template "layout" .}}`))
