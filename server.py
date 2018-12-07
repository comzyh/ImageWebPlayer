#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""web server for ImageWebPlayer."""
import logging
import coloredlogs
import os
import re
import argparse
from aiohttp import web
from functools import cmp_to_key


logger = logging.getLogger('ImageWebPlayer')
coloredlogs.install()


class MainHandler:
    _filename_num_pattern = re.compile(r'^(?P<suffix>^.*?)(?P<sn>\d*)(?P<ext>\..*?)?$')

    def __init__(self, rootdir):
        self.rootdir = rootdir
        self.current_dir = os.path.dirname(os.path.abspath(__file__))
        logger.info("current_dir: {}".format(self.current_dir))

    async def index(self, request):
        return web.FileResponse(os.path.join(self.current_dir, 'index.html'))

    async def listdir(self, request):
        return self.list_impl(request)

    async def listimg(self, request):
        return self.list_impl(request, imgonly=True)

    def list_impl(self, request, imgonly=False):
        path = os.path.join(self.rootdir, request.match_info['path'])
        logger.info('path: {}'.format(path))
        files = []
        dirs = []
        for name in os.listdir(path):
            if os.path.isfile(os.path.join(path, name)):
                if imgonly:
                    ext = name.split('.')[-1]
                    if ext not in ['png', 'jpg', 'jpeg']:
                        continue
                files.append(name)
            elif not imgonly:
                dirs.append(name)

        def filename_compare(x, y):
            gx = self._filename_num_pattern.match(x)
            gy = self._filename_num_pattern.match(y)
            if gx.group('suffix') == gy.group('suffix') and gx.group('ext') == gy.group('ext'):
                snx = gx.group('sn')
                sny = gy.group('sn')
                if int(snx) != int(sny):
                    return int(snx) - int(sny)
            return (x > y) - (x < y)
        files = sorted(files, key=cmp_to_key(filename_compare))
        # files = sorted(files)
        return web.json_response(dict(files=files, dirs=dirs))


def main():
    parser = argparse.ArgumentParser(description='ImageWebPlayer')
    parser.add_argument('root', type=str, help='rootdir to view')
    parser.add_argument('-p', '--port', type=str, default='8080', help='listening port')
    parser.add_argument('-b', '--bind', type=str, default='127.0.0.1', help='bind address')
    args = parser.parse_args()

    rootdir = os.path.abspath(args.root)

    logger.info("root dir: {}".format(rootdir))
    if not os.path.isdir(rootdir):
        logger.error("root dir is invalid.")
        return

    app = web.Application()
    main_handler = MainHandler(rootdir=rootdir)
    app.add_routes([web.get('/', main_handler.index)])
    app.add_routes([web.get('/list/{path:.*}', main_handler.listdir)])
    app.add_routes([web.get('/listimg/{path:.*}', main_handler.listimg)])
    app.add_routes([web.static('/files', rootdir)])

    web.run_app(app, host=args.bind, port=args.port)


if __name__ == '__main__':
    main()
